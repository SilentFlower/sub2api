package antigravity

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"io"
	"strings"
)

const (
	// DefaultGIFFramesPerImage 是单个 GIF 的默认输出帧数上限。
	DefaultGIFFramesPerImage = 8
	// MinGIFFramesPerImage 是单个 GIF 可配置的最小输出帧数。
	MinGIFFramesPerImage = 1
	// MaxGIFFramesPerImage 是单个 GIF 可配置的最大输出帧数。
	MaxGIFFramesPerImage = 16
	// MaxGIFFramesPerRequest 是单次请求由 GIF 转换生成的 PNG part 总数上限。
	MaxGIFFramesPerRequest = 16

	maxGIFDecodedBytes          = 20 * 1024 * 1024
	maxGIFCanvasDimension       = 4096
	maxGIFCanvasPixels          = 16_777_216
	maxGIFSourceFrames          = 1000
	maxGIFSourceFramePixels     = 134_217_728
	maxGIFConvertedRequestBytes = 20 * 1024 * 1024
	gifLogicalScreenMinBytes    = 13
)

var errGIFOutputBudgetExceeded = errors.New("GIF output budget exceeded")

// GIFCompatibilityError 表示可安全返回给客户端的 GIF 兼容转换错误。
type GIFCompatibilityError struct {
	message     string
	diagnostics *GIFCompatibilityDiagnostics
}

// Error 返回不包含原始图片数据的安全错误消息。
//
// @return 可安全暴露给客户端的错误消息。
func (e *GIFCompatibilityError) Error() string {
	if e == nil {
		return "Invalid GIF image"
	}
	return e.message
}

// GIFCompatibilityDiagnostics 表示不包含原始图片数据的 GIF 兼容转换诊断信息。
type GIFCompatibilityDiagnostics struct {
	Stage                string
	InputLength          int
	TrimmedLength        int
	PayloadLength        int
	HasOuterBase64Prefix bool
	HasDataURI           bool
	DataURIMime          string
	DataURIHasBase64     bool
	HadURLEscape         bool
	HasURLSafeAlphabet   bool
	PaddingRemainder     int
	DecodeError          string
}

// IsGIFCompatibilityError 判断错误是否来自 GIF 兼容转换。
//
// @param err 待判断的错误。
// @return 属于 GIF 兼容转换错误时返回 true。
func IsGIFCompatibilityError(err error) bool {
	var target *GIFCompatibilityError
	return errors.As(err, &target)
}

// GIFCompatibilityDiagnosticsFromError 返回 GIF 兼容错误中携带的安全诊断信息。
//
// @param err 待提取诊断信息的错误。
// @return 诊断信息和是否存在诊断信息。
func GIFCompatibilityDiagnosticsFromError(err error) (GIFCompatibilityDiagnostics, bool) {
	var target *GIFCompatibilityError
	if !errors.As(err, &target) || target == nil || target.diagnostics == nil {
		return GIFCompatibilityDiagnostics{}, false
	}
	return *target.diagnostics, true
}

type gifInlineCandidate struct {
	contentIndex int
	partIndex    int
	part         map[string]any
	inline       map[string]any
	inlineKey    string
	mimeKey      string
	dataKey      string
	data         string
	frameCount   int
	allocated    int
}

type gifMetadata struct {
	frameCount int
}

type gifBase64PayloadInfo struct {
	payload    string
	diagnostic GIFCompatibilityDiagnostics
}

type gifOutputBudget struct {
	remainingEncodedBytes int
}

type limitedGIFBuffer struct {
	bytes.Buffer
	limit int
}

// ContainsGIFInlineDataCandidate 快速判断请求体是否可能包含 GIF MIME。
//
// 该函数只做 ASCII 大小写不敏感扫描，允许少量误报，但不能漏掉合法的 image/gif。
//
// @param body Gemini 或 v1internal JSON 请求体。
// @return 请求体可能包含 image/gif 时返回 true。
func ContainsGIFInlineDataCandidate(body []byte) bool {
	const target = "image/gif"
	if len(body) < len(target) {
		return false
	}
	for i := 0; i <= len(body)-len(target); i++ {
		matched := true
		for j := 0; j < len(target); j++ {
			actual := body[i+j]
			if actual >= 'A' && actual <= 'Z' {
				actual += 'a' - 'A'
			}
			if actual != target[j] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// TransformGIFInlineData 将 Gemini 请求中的 GIF inline data 替换为多张 PNG。
//
// 无 GIF 时返回原始字节切片；存在 GIF 时保留非 GIF part 和未知字段的语义。
//
// @param body Gemini 或 v1internal JSON 请求体。
// @param maxFramesPerGIF 单个 GIF 的输出帧数上限，无效值回退为默认值。
// @return 转换后的请求体；GIF 非法或资源超限时返回 GIFCompatibilityError。
func TransformGIFInlineData(body []byte, maxFramesPerGIF int) ([]byte, error) {
	root, err := decodeGIFRequestObject(body)
	if err != nil {
		return nil, newGIFCompatibilityError("Invalid Gemini request body")
	}

	contents := gifRequestContents(root)
	if contents == nil {
		return body, nil
	}

	candidates, err := discoverGIFCandidates(contents)
	if err != nil {
		return nil, err
	}
	if len(candidates) == 0 {
		return body, nil
	}
	if len(candidates) > MaxGIFFramesPerRequest {
		return nil, newGIFCompatibilityError("Too many GIF images in one request")
	}

	for _, candidate := range candidates {
		decoded, decodeErr := decodeGIFBase64(candidate.data)
		if decodeErr != nil {
			return nil, decodeErr
		}
		metadata, inspectErr := inspectGIF(decoded)
		if inspectErr != nil {
			return nil, inspectErr
		}
		candidate.frameCount = metadata.frameCount
	}

	maxFramesPerGIF = normalizeGIFFrameLimit(maxFramesPerGIF)
	allocateGIFFrameBudget(candidates, maxFramesPerGIF)

	replacements := make(map[[2]int][]any, len(candidates))
	outputBudget := &gifOutputBudget{remainingEncodedBytes: maxGIFConvertedRequestBytes}
	for _, candidate := range candidates {
		decoded, decodeErr := decodeGIFBase64(candidate.data)
		if decodeErr != nil {
			return nil, decodeErr
		}
		decodedGIF, decodeAllErr := gif.DecodeAll(bytes.NewReader(decoded))
		if decodeAllErr != nil || len(decodedGIF.Image) != candidate.frameCount {
			return nil, newGIFCompatibilityError("Invalid or corrupted GIF image")
		}

		selectedIndexes := sampleGIFFrameIndexes(candidate.frameCount, candidate.allocated)
		encodedFrames, encodeErr := encodeSelectedGIFFrames(decodedGIF, selectedIndexes, outputBudget)
		if encodeErr != nil {
			return nil, encodeErr
		}

		parts := make([]any, 0, len(encodedFrames))
		for _, encodedFrame := range encodedFrames {
			parts = append(parts, buildGIFReplacementPart(candidate, encodedFrame))
		}
		replacements[[2]int{candidate.contentIndex, candidate.partIndex}] = parts
	}

	applyGIFReplacements(contents, replacements)
	transformed, err := json.Marshal(root)
	if err != nil {
		return nil, newGIFCompatibilityError("Failed to build converted Gemini request")
	}
	if len(transformed) > maxGIFConvertedRequestBytes {
		return nil, newGIFCompatibilityError("Converted GIF request exceeds the 20 MiB inline request limit")
	}
	return transformed, nil
}

func newGIFCompatibilityError(message string) error {
	return &GIFCompatibilityError{message: message}
}

func newGIFCompatibilityErrorWithDiagnostics(message string, diagnostics GIFCompatibilityDiagnostics) error {
	return &GIFCompatibilityError{message: message, diagnostics: &diagnostics}
}

func decodeGIFRequestObject(body []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()

	var root map[string]any
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("request root is not an object")
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("request contains multiple JSON values")
		}
		return nil, err
	}
	return root, nil
}

func gifRequestContents(root map[string]any) []any {
	if request, ok := root["request"].(map[string]any); ok {
		if contents, ok := request["contents"].([]any); ok {
			return contents
		}
	}
	contents, _ := root["contents"].([]any)
	return contents
}

func discoverGIFCandidates(contents []any) ([]*gifInlineCandidate, error) {
	candidates := make([]*gifInlineCandidate, 0)
	for contentIndex, contentValue := range contents {
		content, ok := contentValue.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}
		for partIndex, partValue := range parts {
			part, ok := partValue.(map[string]any)
			if !ok {
				continue
			}
			candidate, candidateErr := gifCandidateFromPart(contentIndex, partIndex, part)
			if candidateErr != nil {
				return nil, candidateErr
			}
			if candidate != nil {
				candidates = append(candidates, candidate)
			}
		}
	}
	return candidates, nil
}

func gifCandidateFromPart(contentIndex, partIndex int, part map[string]any) (*gifInlineCandidate, error) {
	inlineKey, inline := gifInlineDataObject(part)
	if inline == nil {
		return nil, nil
	}

	mimeKey, mimeType := gifStringField(inline, "mimeType", "mime_type")
	if !strings.EqualFold(strings.TrimSpace(mimeType), "image/gif") {
		return nil, nil
	}
	dataKey, data := gifStringField(inline, "data")
	if dataKey == "" || strings.TrimSpace(data) == "" {
		return nil, newGIFCompatibilityError("GIF inline data is missing base64 content")
	}

	return &gifInlineCandidate{
		contentIndex: contentIndex,
		partIndex:    partIndex,
		part:         part,
		inline:       inline,
		inlineKey:    inlineKey,
		mimeKey:      mimeKey,
		dataKey:      dataKey,
		data:         data,
	}, nil
}

func gifInlineDataObject(part map[string]any) (string, map[string]any) {
	for _, key := range []string{"inlineData", "inline_data"} {
		if inline, ok := part[key].(map[string]any); ok {
			return key, inline
		}
	}
	return "", nil
}

func gifStringField(value map[string]any, keys ...string) (string, string) {
	for _, key := range keys {
		if field, ok := value[key].(string); ok {
			return key, field
		}
	}
	return "", ""
}

func normalizeGIFFrameLimit(value int) int {
	if value < MinGIFFramesPerImage || value > MaxGIFFramesPerImage {
		return DefaultGIFFramesPerImage
	}
	return value
}

func allocateGIFFrameBudget(candidates []*gifInlineCandidate, maxFramesPerGIF int) {
	remaining := MaxGIFFramesPerRequest
	for _, candidate := range candidates {
		candidate.allocated = 1
		remaining--
	}

	for remaining > 0 {
		progressed := false
		for _, candidate := range candidates {
			target := candidate.frameCount
			if target > maxFramesPerGIF {
				target = maxFramesPerGIF
			}
			if candidate.allocated >= target {
				continue
			}
			candidate.allocated++
			remaining--
			progressed = true
			if remaining == 0 {
				break
			}
		}
		if !progressed {
			break
		}
	}
}

func sampleGIFFrameIndexes(frameCount, selectedCount int) []int {
	if frameCount <= 0 || selectedCount <= 0 {
		return nil
	}
	if selectedCount >= frameCount {
		indexes := make([]int, frameCount)
		for index := range indexes {
			indexes[index] = index
		}
		return indexes
	}
	if selectedCount == 1 {
		return []int{0}
	}

	indexes := make([]int, selectedCount)
	denominator := selectedCount - 1
	for index := 0; index < selectedCount; index++ {
		numerator := index * (frameCount - 1)
		indexes[index] = (numerator + denominator/2) / denominator
	}
	return indexes
}

func decodeGIFBase64(value string) ([]byte, error) {
	payloadInfo, err := gifBase64Payload(value)
	if err != nil {
		return nil, err
	}
	diagnostic := payloadInfo.diagnostic

	diagnostic.PayloadLength = len(payloadInfo.payload)
	diagnostic.HadURLEscape = strings.Contains(payloadInfo.payload, "%")
	diagnostic.HasURLSafeAlphabet = hasGIFBase64URLSafeAlphabet(payloadInfo.payload)
	diagnostic.PaddingRemainder = len(payloadInfo.payload) % 4

	maxEncodedLength := base64.StdEncoding.EncodedLen(maxGIFDecodedBytes)
	if len(payloadInfo.payload) > maxEncodedLength+2 {
		diagnostic.Stage = "input_limit"
		return nil, newGIFCompatibilityErrorWithDiagnostics("GIF image exceeds the 20 MiB input limit", diagnostic)
	}

	decoded, decodeErr := base64.StdEncoding.DecodeString(payloadInfo.payload)
	if decodeErr != nil {
		decoded, decodeErr = base64.RawStdEncoding.DecodeString(payloadInfo.payload)
	}
	if decodeErr != nil {
		diagnostic.Stage = "base64_decode"
		diagnostic.DecodeError = decodeErr.Error()
		return nil, newGIFCompatibilityErrorWithDiagnostics("Invalid GIF base64 data", diagnostic)
	}
	if len(decoded) > maxGIFDecodedBytes {
		diagnostic.Stage = "input_limit"
		return nil, newGIFCompatibilityErrorWithDiagnostics("GIF image exceeds the 20 MiB input limit", diagnostic)
	}
	return decoded, nil
}

func hasGIFBase64URLSafeAlphabet(value string) bool {
	return strings.ContainsAny(value, "-_")
}

func gifBase64Payload(value string) (gifBase64PayloadInfo, error) {
	trimmed := strings.TrimSpace(value)
	diagnostic := GIFCompatibilityDiagnostics{
		Stage:         "payload_parse",
		InputLength:   len(value),
		TrimmedLength: len(trimmed),
	}
	if hasASCIIPrefixFold(trimmed, "base64:") {
		// 部分客户端会把 data URI 再包装为 base64: URL；只剥离一层，避免接受无界嵌套格式。
		diagnostic.HasOuterBase64Prefix = true
		trimmed = strings.TrimSpace(trimmed[len("base64:"):])
		if trimmed == "" {
			return gifBase64PayloadInfo{}, newGIFCompatibilityErrorWithDiagnostics("Invalid GIF base64 data", diagnostic)
		}
	}
	if !hasASCIIPrefixFold(trimmed, "data:") {
		return gifBase64PayloadInfo{payload: trimmed, diagnostic: diagnostic}, nil
	}

	diagnostic.HasDataURI = true
	commaIndex := strings.IndexByte(trimmed, ',')
	if commaIndex < 0 {
		return gifBase64PayloadInfo{}, newGIFCompatibilityErrorWithDiagnostics("Invalid GIF data URI", diagnostic)
	}
	metadata := strings.Split(trimmed[len("data:"):commaIndex], ";")
	if len(metadata) > 0 {
		diagnostic.DataURIMime = truncateGIFDiagnosticValue(strings.TrimSpace(metadata[0]))
	}
	if len(metadata) < 2 || !strings.EqualFold(strings.TrimSpace(metadata[0]), "image/gif") {
		return gifBase64PayloadInfo{}, newGIFCompatibilityErrorWithDiagnostics("Invalid GIF data URI", diagnostic)
	}
	hasBase64 := false
	for _, item := range metadata[1:] {
		if strings.EqualFold(strings.TrimSpace(item), "base64") {
			hasBase64 = true
			break
		}
	}
	diagnostic.DataURIHasBase64 = hasBase64
	if !hasBase64 {
		return gifBase64PayloadInfo{}, newGIFCompatibilityErrorWithDiagnostics("Invalid GIF data URI", diagnostic)
	}
	return gifBase64PayloadInfo{
		payload:    strings.TrimSpace(trimmed[commaIndex+1:]),
		diagnostic: diagnostic,
	}, nil
}

func truncateGIFDiagnosticValue(value string) string {
	if len(value) <= 80 {
		return value
	}
	return value[:80]
}

func hasASCIIPrefixFold(value, prefix string) bool {
	if len(value) < len(prefix) {
		return false
	}
	return strings.EqualFold(value[:len(prefix)], prefix)
}

func inspectGIF(data []byte) (gifMetadata, error) {
	if len(data) < gifLogicalScreenMinBytes {
		return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
	}
	if string(data[:6]) != "GIF87a" && string(data[:6]) != "GIF89a" {
		return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
	}

	canvasWidth := int(binary.LittleEndian.Uint16(data[6:8]))
	canvasHeight := int(binary.LittleEndian.Uint16(data[8:10]))
	if canvasWidth <= 0 || canvasHeight <= 0 ||
		canvasWidth > maxGIFCanvasDimension || canvasHeight > maxGIFCanvasDimension ||
		uint64(canvasWidth)*uint64(canvasHeight) > maxGIFCanvasPixels {
		return gifMetadata{}, newGIFCompatibilityError("GIF image exceeds the supported dimensions")
	}

	offset := gifLogicalScreenMinBytes
	globalPacked := data[10]
	if globalPacked&0x80 != 0 {
		colorTableBytes := 3 * (1 << (int(globalPacked&0x07) + 1))
		if offset+colorTableBytes > len(data) {
			return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
		}
		offset += colorTableBytes
	}

	frameCount := 0
	var framePixels uint64
	for offset < len(data) {
		blockType := data[offset]
		offset++
		switch blockType {
		case 0x3B:
			if frameCount == 0 {
				return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
			}
			return gifMetadata{frameCount: frameCount}, nil

		case 0x21:
			if offset >= len(data) {
				return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
			}
			offset++
			var skipErr error
			offset, skipErr = skipGIFSubBlocks(data, offset)
			if skipErr != nil {
				return gifMetadata{}, skipErr
			}

		case 0x2C:
			if offset+9 > len(data) {
				return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
			}
			left := int(binary.LittleEndian.Uint16(data[offset : offset+2]))
			top := int(binary.LittleEndian.Uint16(data[offset+2 : offset+4]))
			width := int(binary.LittleEndian.Uint16(data[offset+4 : offset+6]))
			height := int(binary.LittleEndian.Uint16(data[offset+6 : offset+8]))
			packed := data[offset+8]
			offset += 9

			if width <= 0 || height <= 0 || left+width > canvasWidth || top+height > canvasHeight {
				return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
			}
			frameCount++
			if frameCount > maxGIFSourceFrames {
				return gifMetadata{}, newGIFCompatibilityError("GIF image exceeds the supported frame limit")
			}
			framePixels += uint64(width) * uint64(height)
			if framePixels > maxGIFSourceFramePixels {
				return gifMetadata{}, newGIFCompatibilityError("GIF image exceeds the supported pixel limit")
			}

			if packed&0x80 != 0 {
				colorTableBytes := 3 * (1 << (int(packed&0x07) + 1))
				if offset+colorTableBytes > len(data) {
					return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
				}
				offset += colorTableBytes
			}
			if offset >= len(data) {
				return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
			}
			offset++
			var skipErr error
			offset, skipErr = skipGIFSubBlocks(data, offset)
			if skipErr != nil {
				return gifMetadata{}, skipErr
			}

		default:
			return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
		}
	}

	return gifMetadata{}, newGIFCompatibilityError("Invalid or corrupted GIF image")
}

func skipGIFSubBlocks(data []byte, offset int) (int, error) {
	for {
		if offset >= len(data) {
			return 0, newGIFCompatibilityError("Invalid or corrupted GIF image")
		}
		blockSize := int(data[offset])
		offset++
		if blockSize == 0 {
			return offset, nil
		}
		if offset+blockSize > len(data) {
			return 0, newGIFCompatibilityError("Invalid or corrupted GIF image")
		}
		offset += blockSize
	}
}

func encodeSelectedGIFFrames(decodedGIF *gif.GIF, selectedIndexes []int, outputBudget *gifOutputBudget) ([]string, error) {
	if decodedGIF == nil || len(decodedGIF.Image) == 0 {
		return nil, newGIFCompatibilityError("Invalid or corrupted GIF image")
	}

	canvasBounds := image.Rect(0, 0, decodedGIF.Config.Width, decodedGIF.Config.Height)
	canvas := image.NewRGBA(canvasBounds)
	background := gifBackgroundColor(decodedGIF)
	draw.Draw(canvas, canvasBounds, image.NewUniform(background), image.Point{}, draw.Src)

	selected := make(map[int]struct{}, len(selectedIndexes))
	for _, index := range selectedIndexes {
		selected[index] = struct{}{}
	}
	encodedFrames := make([]string, 0, len(selectedIndexes))

	for index, frame := range decodedGIF.Image {
		disposal := byte(gif.DisposalNone)
		if index < len(decodedGIF.Disposal) {
			disposal = decodedGIF.Disposal[index]
		}

		var previousCanvas *image.RGBA
		if disposal == gif.DisposalPrevious {
			previousCanvas = cloneGIFCanvas(canvas)
		}

		frameBounds := frame.Bounds().Intersect(canvasBounds)
		draw.Draw(canvas, frameBounds, frame, frameBounds.Min, draw.Over)
		if _, ok := selected[index]; ok {
			encodedFrame, err := encodeGIFCanvasWithinBudget(canvas, outputBudget)
			if err != nil {
				return nil, err
			}
			encodedFrames = append(encodedFrames, encodedFrame)
		}

		switch disposal {
		case gif.DisposalBackground:
			draw.Draw(canvas, frameBounds, image.NewUniform(background), image.Point{}, draw.Src)
		case gif.DisposalPrevious:
			if previousCanvas != nil {
				draw.Draw(canvas, canvasBounds, previousCanvas, canvasBounds.Min, draw.Src)
			}
		}
	}

	if len(encodedFrames) != len(selectedIndexes) {
		return nil, newGIFCompatibilityError("Invalid or corrupted GIF image")
	}
	return encodedFrames, nil
}

func encodeGIFCanvasWithinBudget(canvas image.Image, outputBudget *gifOutputBudget) (string, error) {
	if outputBudget == nil || outputBudget.remainingEncodedBytes <= 0 {
		return "", newGIFCompatibilityError("Converted GIF request exceeds the 20 MiB inline request limit")
	}

	// PNG 先受原始字节预算约束，避免编码完成后才发现 base64 已经无法装入请求。
	rawLimit := maxDecodedBytesForBase64Budget(outputBudget.remainingEncodedBytes)
	if rawLimit <= 0 {
		return "", newGIFCompatibilityError("Converted GIF request exceeds the 20 MiB inline request limit")
	}
	buffer := &limitedGIFBuffer{limit: rawLimit}
	if err := png.Encode(buffer, canvas); err != nil {
		if errors.Is(err, errGIFOutputBudgetExceeded) {
			return "", newGIFCompatibilityError("Converted GIF request exceeds the 20 MiB inline request limit")
		}
		return "", newGIFCompatibilityError("Failed to convert GIF frame to PNG")
	}

	encodedFrame := base64.StdEncoding.EncodeToString(buffer.Bytes())
	if len(encodedFrame) > outputBudget.remainingEncodedBytes {
		return "", newGIFCompatibilityError("Converted GIF request exceeds the 20 MiB inline request limit")
	}
	outputBudget.remainingEncodedBytes -= len(encodedFrame)
	return encodedFrame, nil
}

func maxDecodedBytesForBase64Budget(encodedBudget int) int {
	decodedLimit := encodedBudget / 4 * 3
	for decodedLimit > 0 && base64.StdEncoding.EncodedLen(decodedLimit) > encodedBudget {
		decodedLimit--
	}
	return decodedLimit
}

func (b *limitedGIFBuffer) Write(data []byte) (int, error) {
	if b == nil || len(data) > b.limit-b.Len() {
		return 0, errGIFOutputBudgetExceeded
	}
	return b.Buffer.Write(data)
}

func gifBackgroundColor(decodedGIF *gif.GIF) color.Color {
	palette, ok := decodedGIF.Config.ColorModel.(color.Palette)
	if !ok || int(decodedGIF.BackgroundIndex) >= len(palette) {
		return color.Transparent
	}
	return palette[decodedGIF.BackgroundIndex]
}

func cloneGIFCanvas(source *image.RGBA) *image.RGBA {
	clone := image.NewRGBA(source.Bounds())
	draw.Draw(clone, clone.Bounds(), source, source.Bounds().Min, draw.Src)
	return clone
}

func buildGIFReplacementPart(candidate *gifInlineCandidate, encodedFrame string) map[string]any {
	part := cloneGIFMap(candidate.part)
	inline := cloneGIFMap(candidate.inline)
	inline[candidate.mimeKey] = "image/png"
	inline[candidate.dataKey] = encodedFrame
	part[candidate.inlineKey] = inline
	return part
}

func cloneGIFMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func applyGIFReplacements(contents []any, replacements map[[2]int][]any) {
	for contentIndex, contentValue := range contents {
		content, ok := contentValue.(map[string]any)
		if !ok {
			continue
		}
		parts, ok := content["parts"].([]any)
		if !ok {
			continue
		}

		newParts := make([]any, 0, len(parts))
		for partIndex, part := range parts {
			if replacement, ok := replacements[[2]int{contentIndex, partIndex}]; ok {
				newParts = append(newParts, replacement...)
				continue
			}
			newParts = append(newParts, part)
		}
		content["parts"] = newParts
	}
}
