//go:build unit

package antigravity

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTransformGIFInlineData_NoGIFReturnsOriginalBytes(t *testing.T) {
	body := []byte(`{"contents":[{"role":"user","parts":[{"inlineData":{"mimeType":"image/png","data":"abc"}},{"text":"保留"}]}]}`)

	transformed, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.NoError(t, err)
	require.Equal(t, body, transformed)
}

func TestTransformGIFInlineData_SingleFrameDataURI(t *testing.T) {
	gifData := encodeSolidTestGIF(t, []color.RGBA{{R: 255, A: 255}})
	body := buildGIFRequestBody(t, []any{
		map[string]any{
			"inlineData": map[string]any{
				"mimeType": "image/gif",
				"data":     "data:image/gif;base64," + base64.StdEncoding.EncodeToString(gifData),
			},
		},
	})

	transformed, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.NoError(t, err)
	parts := requestParts(t, transformed)
	require.Len(t, parts, 1)
	inline := parts[0]["inlineData"].(map[string]any)
	require.Equal(t, "image/png", inline["mimeType"])
	require.NotContains(t, inline["data"].(string), "data:image")
	require.Equal(t, color.RGBA{R: 255, A: 255}, pngPixel(t, inline["data"].(string), 0, 0))
}

func TestTransformGIFInlineData_SupportsBase64WrappedDataURI(t *testing.T) {
	gifData := encodeSolidTestGIF(t, []color.RGBA{{G: 255, A: 255}})
	body := buildGIFRequestBody(t, []any{
		map[string]any{
			"inlineData": map[string]any{
				"mimeType": "image/gif",
				"data":     "base64:data:image/gif;base64," + base64.StdEncoding.EncodeToString(gifData),
			},
		},
	})

	transformed, err := TransformGIFInlineData(body, 1)

	require.NoError(t, err)
	parts := requestParts(t, transformed)
	require.Len(t, parts, 1)
	inline := parts[0]["inlineData"].(map[string]any)
	require.Equal(t, "image/png", inline["mimeType"])
	require.Equal(t, color.RGBA{G: 255, A: 255}, pngPixel(t, inline["data"].(string), 0, 0))
}

func TestTransformGIFInlineData_SupportsEscapedAndURLSafeBase64(t *testing.T) {
	gifData := encodeSolidTestGIF(t, []color.RGBA{{B: 255, A: 255}})
	tests := []struct {
		name string
		data string
	}{
		{
			name: "URL 转义 data URI",
			data: "data:image/gif;base64," + url.PathEscape(base64.StdEncoding.EncodeToString(gifData)),
		},
		{
			name: "URL-safe base64",
			data: "data:image/gif;base64," + base64.RawURLEncoding.EncodeToString(gifData),
		},
		{
			name: "带空白的 base64",
			data: "base64:data:image/gif;base64," + spacedGIFBase64(base64.StdEncoding.EncodeToString(gifData)),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := buildGIFRequestBody(t, []any{
				map[string]any{
					"inlineData": map[string]any{
						"mimeType": "image/gif",
						"data":     test.data,
					},
				},
			})

			transformed, err := TransformGIFInlineData(body, 1)

			require.NoError(t, err)
			parts := requestParts(t, transformed)
			require.Len(t, parts, 1)
			inline := parts[0]["inlineData"].(map[string]any)
			require.Equal(t, "image/png", inline["mimeType"])
			require.Equal(t, color.RGBA{B: 255, A: 255}, pngPixel(t, inline["data"].(string), 0, 0))
		})
	}
}

func TestTransformGIFInlineData_DefaultSamplingKeepsFirstAndLast(t *testing.T) {
	colors := make([]color.RGBA, 10)
	for index := range colors {
		colors[index] = color.RGBA{R: uint8(index * 20), G: uint8(255 - index*20), A: 255}
	}
	body := buildGIFRequestBody(t, []any{gifPart(encodeSolidTestGIF(t, colors))})

	transformed, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.NoError(t, err)
	parts := requestParts(t, transformed)
	require.Len(t, parts, DefaultGIFFramesPerImage)
	require.Equal(t, colors[0], partPNGPixel(t, parts[0], 0, 0))
	require.Equal(t, colors[len(colors)-1], partPNGPixel(t, parts[len(parts)-1], 0, 0))
	require.Equal(t, []int{0, 1, 3, 4, 5, 6, 8, 9}, sampleGIFFrameIndexes(10, 8))
}

func TestTransformGIFInlineData_MultipleGIFsShareBudgetFairly(t *testing.T) {
	colors := make([]color.RGBA, 10)
	for index := range colors {
		colors[index] = color.RGBA{R: uint8(index * 10), B: uint8(255 - index*10), A: 255}
	}
	gifData := encodeSolidTestGIF(t, colors)
	body := buildGIFRequestBody(t, []any{
		gifPart(gifData),
		map[string]any{"text": "first-boundary"},
		gifPart(gifData),
		map[string]any{"text": "second-boundary"},
		gifPart(gifData),
	})

	transformed, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.NoError(t, err)
	parts := requestParts(t, transformed)
	require.Equal(t, []int{6, 5, 5}, pngGroupSizes(parts))
}

func TestTransformGIFInlineData_SupportsWrappedSnakeCase(t *testing.T) {
	gifData := encodeSolidTestGIF(t, []color.RGBA{{G: 255, A: 255}})
	body, err := json.Marshal(map[string]any{
		"project": "project",
		"request": map[string]any{
			"contents": []any{
				map[string]any{
					"parts": []any{
						map[string]any{
							"inline_data": map[string]any{
								"mime_type": "IMAGE/GIF",
								"data":      base64.StdEncoding.EncodeToString(gifData),
							},
						},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	transformed, err := TransformGIFInlineData(body, 1)

	require.NoError(t, err)
	parts := requestParts(t, transformed)
	require.Len(t, parts, 1)
	inline := parts[0]["inline_data"].(map[string]any)
	require.Equal(t, "image/png", inline["mime_type"])
}

func TestTransformGIFInlineData_DisposalBackgroundClearsFrameRectangle(t *testing.T) {
	gifData := encodeDisposalTestGIF(t, gif.DisposalBackground)
	body := buildGIFRequestBody(t, []any{gifPart(gifData)})

	transformed, err := TransformGIFInlineData(body, 3)

	require.NoError(t, err)
	parts := requestParts(t, transformed)
	require.Len(t, parts, 3)
	require.Equal(t, color.RGBA{A: 255}, partPNGPixel(t, parts[2], 0, 0))
	require.Equal(t, color.RGBA{G: 255, A: 255}, partPNGPixel(t, parts[2], 1, 0))
}

func TestTransformGIFInlineData_DisposalPreviousRestoresCanvas(t *testing.T) {
	gifData := encodeDisposalTestGIF(t, gif.DisposalPrevious)
	body := buildGIFRequestBody(t, []any{gifPart(gifData)})

	transformed, err := TransformGIFInlineData(body, 3)

	require.NoError(t, err)
	parts := requestParts(t, transformed)
	require.Len(t, parts, 3)
	require.Equal(t, color.RGBA{R: 255, A: 255}, partPNGPixel(t, parts[2], 0, 0))
	require.Equal(t, color.RGBA{G: 255, A: 255}, partPNGPixel(t, parts[2], 1, 0))
}

func TestTransformGIFInlineData_TransparentPixelsPreservePreviousCanvas(t *testing.T) {
	body := buildGIFRequestBody(t, []any{gifPart(encodeTransparentOverlayTestGIF(t))})

	transformed, err := TransformGIFInlineData(body, 2)

	require.NoError(t, err)
	parts := requestParts(t, transformed)
	require.Len(t, parts, 2)
	require.Equal(t, color.RGBA{R: 255, A: 255}, partPNGPixel(t, parts[1], 0, 0))
	require.Equal(t, color.RGBA{B: 255, A: 255}, partPNGPixel(t, parts[1], 1, 0))
}

func TestTransformGIFInlineData_PreservesMixedPartsAndUnknownFields(t *testing.T) {
	nonGIFPart := map[string]any{
		"inlineData": map[string]any{
			"mimeType": "image/png",
			"data":     "png-data",
			"name":     "keep-inline",
		},
		"metadata": "keep-part",
	}
	gifPartWithMetadata := gifPart(encodeSolidTestGIF(t, []color.RGBA{{R: 255, A: 255}}))
	gifPartWithMetadata["metadata"] = "keep-gif-part"
	gifPartWithMetadata["inlineData"].(map[string]any)["name"] = "keep-gif-inline"
	body := buildGIFRequestBody(t, []any{
		map[string]any{"text": "before"},
		nonGIFPart,
		gifPartWithMetadata,
		map[string]any{"text": "after"},
	})

	transformed, err := TransformGIFInlineData(body, 1)

	require.NoError(t, err)
	parts := requestParts(t, transformed)
	require.Len(t, parts, 4)
	require.Equal(t, map[string]any{"text": "before"}, parts[0])
	require.Equal(t, nonGIFPart, parts[1])
	require.Equal(t, "keep-gif-part", parts[2]["metadata"])
	require.Equal(t, "keep-gif-inline", parts[2]["inlineData"].(map[string]any)["name"])
	require.Equal(t, map[string]any{"text": "after"}, parts[3])
}

func TestTransformGIFInlineData_RejectsInvalidBase64(t *testing.T) {
	body := buildGIFRequestBody(t, []any{
		map[string]any{"inlineData": map[string]any{"mimeType": "image/gif", "data": "%%%"}},
	})

	_, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.Error(t, err)
	require.True(t, IsGIFCompatibilityError(err))
	require.Equal(t, "Invalid GIF base64 data", err.Error())
}

func TestTransformGIFInlineData_InvalidBase64ExposesSafeDiagnostics(t *testing.T) {
	body := buildGIFRequestBody(t, []any{
		map[string]any{
			"inlineData": map[string]any{
				"mimeType": "image/gif",
				"data":     "base64:data:image/gif;base64,%%%---",
			},
		},
	})

	_, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.Error(t, err)
	diagnostic, ok := GIFCompatibilityDiagnosticsFromError(err)
	require.True(t, ok)
	require.Equal(t, "base64_decode", diagnostic.Stage)
	require.True(t, diagnostic.HasOuterBase64Prefix)
	require.True(t, diagnostic.HasDataURI)
	require.Equal(t, "image/gif", diagnostic.DataURIMime)
	require.True(t, diagnostic.DataURIHasBase64)
	require.True(t, diagnostic.URLUnescapeFailed)
	require.True(t, diagnostic.HasURLSafeAlphabet)
	require.NotEmpty(t, diagnostic.DecodeError)
	require.NotContains(t, diagnostic.DecodeError, "%%%---")
}

func TestTransformGIFInlineData_RejectsMismatchedDataURI(t *testing.T) {
	gifData := encodeSolidTestGIF(t, []color.RGBA{{B: 255, A: 255}})
	body := buildGIFRequestBody(t, []any{
		map[string]any{
			"inlineData": map[string]any{
				"mimeType": "image/gif",
				"data":     "data:image/png;base64," + base64.StdEncoding.EncodeToString(gifData),
			},
		},
	})

	_, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.Error(t, err)
	require.Equal(t, "Invalid GIF data URI", err.Error())
}

func TestTransformGIFInlineData_RejectsTooManyGIFs(t *testing.T) {
	gifData := encodeSolidTestGIF(t, []color.RGBA{{R: 255, A: 255}})
	parts := make([]any, MaxGIFFramesPerRequest+1)
	for index := range parts {
		parts[index] = gifPart(gifData)
	}

	_, err := TransformGIFInlineData(buildGIFRequestBody(t, parts), DefaultGIFFramesPerImage)

	require.Error(t, err)
	require.Equal(t, "Too many GIF images in one request", err.Error())
}

func TestTransformGIFInlineData_RejectsOversizedCanvasBeforeDecodeAll(t *testing.T) {
	gifData := encodeSolidTestGIF(t, []color.RGBA{{R: 255, A: 255}})
	binary.LittleEndian.PutUint16(gifData[6:8], maxGIFCanvasDimension+1)
	body := buildGIFRequestBody(t, []any{gifPart(gifData)})

	_, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.Error(t, err)
	require.Equal(t, "GIF image exceeds the supported dimensions", err.Error())
}

func TestDecodeGIFBase64_RejectsDecodedInputOverLimit(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(make([]byte, maxGIFDecodedBytes+1))

	_, err := decodeGIFBase64(encoded)

	require.Error(t, err)
	require.Equal(t, "GIF image exceeds the 20 MiB input limit", err.Error())
}

func TestTransformGIFInlineData_RejectsCorruptedGIF(t *testing.T) {
	gifData := encodeSolidTestGIF(t, []color.RGBA{{R: 255, A: 255}})
	body := buildGIFRequestBody(t, []any{gifPart(gifData[:len(gifData)-2])})

	_, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.Error(t, err)
	require.Equal(t, "Invalid or corrupted GIF image", err.Error())
}

func TestTransformGIFInlineData_RejectsSourceFrameCountOverLimit(t *testing.T) {
	body := buildGIFRequestBody(t, []any{gifPart(encodeRepeatedTestGIF(t, maxGIFSourceFrames+1))})

	_, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.Error(t, err)
	require.Equal(t, "GIF image exceeds the supported frame limit", err.Error())
}

func TestTransformGIFInlineData_RejectsCumulativeFramePixelsOverLimit(t *testing.T) {
	body := buildGIFRequestBody(t, []any{gifPart(encodeOversizedCumulativePixelsTestGIF(t))})

	_, err := TransformGIFInlineData(body, DefaultGIFFramesPerImage)

	require.Error(t, err)
	require.Equal(t, "GIF image exceeds the supported pixel limit", err.Error())
}

func TestEncodeSelectedGIFFrames_RejectsOutputBudgetOverLimit(t *testing.T) {
	decodedGIF, err := gif.DecodeAll(bytes.NewReader(encodeSolidTestGIF(t, []color.RGBA{{R: 255, A: 255}})))
	require.NoError(t, err)

	_, err = encodeSelectedGIFFrames(decodedGIF, []int{0}, &gifOutputBudget{remainingEncodedBytes: 1})

	require.Error(t, err)
	require.Equal(t, "Converted GIF request exceeds the 20 MiB inline request limit", err.Error())
}

func gifPart(data []byte) map[string]any {
	return map[string]any{
		"inlineData": map[string]any{
			"mimeType": "image/gif",
			"data":     base64.StdEncoding.EncodeToString(data),
		},
	}
}

func buildGIFRequestBody(t *testing.T, parts []any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"contents": []any{
			map[string]any{"role": "user", "parts": parts},
		},
	})
	require.NoError(t, err)
	return body
}

func encodeSolidTestGIF(t *testing.T, colors []color.RGBA) []byte {
	t.Helper()
	palette := make(color.Palette, len(colors)+1)
	palette[0] = color.RGBA{}
	for index, frameColor := range colors {
		palette[index+1] = frameColor
	}
	frames := make([]*image.Paletted, len(colors))
	for index := range colors {
		frame := image.NewPaletted(image.Rect(0, 0, 1, 1), palette)
		frame.SetColorIndex(0, 0, uint8(index+1))
		frames[index] = frame
	}
	return encodeTestGIF(t, 1, 1, palette, frames, nil)
}

func encodeDisposalTestGIF(t *testing.T, middleDisposal byte) []byte {
	t.Helper()
	palette := color.Palette{
		color.RGBA{},
		color.RGBA{R: 255, A: 255},
		color.RGBA{B: 255, A: 255},
		color.RGBA{G: 255, A: 255},
	}

	first := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	first.SetColorIndex(0, 0, 1)
	first.SetColorIndex(1, 0, 1)
	middle := image.NewPaletted(image.Rect(0, 0, 1, 1), palette)
	middle.SetColorIndex(0, 0, 2)
	last := image.NewPaletted(image.Rect(1, 0, 2, 1), palette)
	last.SetColorIndex(1, 0, 3)

	return encodeTestGIF(t, 2, 1, palette, []*image.Paletted{first, middle, last}, []byte{
		gif.DisposalNone,
		middleDisposal,
		gif.DisposalNone,
	})
}

func encodeTransparentOverlayTestGIF(t *testing.T) []byte {
	t.Helper()
	palette := color.Palette{
		color.RGBA{},
		color.RGBA{R: 255, A: 255},
		color.RGBA{B: 255, A: 255},
	}
	first := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	first.SetColorIndex(0, 0, 1)
	first.SetColorIndex(1, 0, 1)
	second := image.NewPaletted(image.Rect(0, 0, 2, 1), palette)
	second.SetColorIndex(0, 0, 0)
	second.SetColorIndex(1, 0, 2)
	return encodeTestGIF(t, 2, 1, palette, []*image.Paletted{first, second}, []byte{
		gif.DisposalNone,
		gif.DisposalNone,
	})
}

func encodeRepeatedTestGIF(t *testing.T, frameCount int) []byte {
	t.Helper()
	palette := color.Palette{color.RGBA{}, color.RGBA{R: 255, A: 255}}
	frames := make([]*image.Paletted, frameCount)
	for index := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, 1, 1), palette)
		frame.SetColorIndex(0, 0, 1)
		frames[index] = frame
	}
	return encodeTestGIF(t, 1, 1, palette, frames, nil)
}

func encodeOversizedCumulativePixelsTestGIF(t *testing.T) []byte {
	t.Helper()
	data := encodeSolidTestGIF(t, []color.RGBA{{R: 255, A: 255}})
	descriptorOffset := bytes.IndexByte(data[gifLogicalScreenMinBytes:], 0x2C)
	require.NotEqual(t, -1, descriptorOffset)
	descriptorOffset += gifLogicalScreenMinBytes
	require.Equal(t, byte(0x3B), data[len(data)-1])

	prefix := append([]byte(nil), data[:descriptorOffset]...)
	binary.LittleEndian.PutUint16(prefix[6:8], maxGIFCanvasDimension)
	binary.LittleEndian.PutUint16(prefix[8:10], maxGIFCanvasDimension)
	frameBlock := append([]byte(nil), data[descriptorOffset:len(data)-1]...)
	binary.LittleEndian.PutUint16(frameBlock[5:7], maxGIFCanvasDimension)
	binary.LittleEndian.PutUint16(frameBlock[7:9], maxGIFCanvasDimension)

	result := prefix
	for range 9 {
		result = append(result, frameBlock...)
	}
	return append(result, 0x3B)
}

func encodeTestGIF(t *testing.T, width, height int, palette color.Palette, frames []*image.Paletted, disposal []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	err := gif.EncodeAll(&buffer, &gif.GIF{
		Image:           frames,
		Delay:           make([]int, len(frames)),
		Disposal:        disposal,
		BackgroundIndex: 0,
		Config: image.Config{
			ColorModel: palette,
			Width:      width,
			Height:     height,
		},
	})
	require.NoError(t, err)
	return buffer.Bytes()
}

func spacedGIFBase64(encoded string) string {
	if len(encoded) < 8 {
		return encoded
	}
	return strings.Join([]string{encoded[:4], encoded[4:8], encoded[8:]}, " \n\t")
}

func requestParts(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var root map[string]any
	require.NoError(t, json.Unmarshal(body, &root))
	request := root
	if wrapped, ok := root["request"].(map[string]any); ok {
		request = wrapped
	}
	contents := request["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	result := make([]map[string]any, 0, len(parts))
	for _, part := range parts {
		result = append(result, part.(map[string]any))
	}
	return result
}

func pngGroupSizes(parts []map[string]any) []int {
	groups := make([]int, 1)
	for _, part := range parts {
		if _, ok := part["text"]; ok {
			groups = append(groups, 0)
			continue
		}
		groups[len(groups)-1]++
	}
	return groups
}

func partPNGPixel(t *testing.T, part map[string]any, x, y int) color.RGBA {
	t.Helper()
	inline := part["inlineData"].(map[string]any)
	return pngPixel(t, inline["data"].(string), x, y)
}

func pngPixel(t *testing.T, encoded string, x, y int) color.RGBA {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(encoded)
	require.NoError(t, err)
	decoded, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	return color.RGBAModel.Convert(decoded.At(x, y)).(color.RGBA)
}
