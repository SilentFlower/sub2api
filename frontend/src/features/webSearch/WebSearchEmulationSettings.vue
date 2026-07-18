<template>
  <div class="card">
    <div class="border-b border-gray-100 px-6 py-4 dark:border-dark-700">
      <h2 class="text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.webSearchEmulation.title") }}
      </h2>
      <p class="mt-1 text-sm text-gray-500 dark:text-gray-400">
        {{ t("admin.settings.webSearchEmulation.description") }}
      </p>
    </div>
    <div class="space-y-5 p-6">
      <div class="flex items-center justify-between">
        <div>
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.webSearchEmulation.enabled") }}
          </label>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ t("admin.settings.webSearchEmulation.enabledHint") }}
          </p>
        </div>
        <Toggle v-model="webSearchConfig.enabled" />
      </div>

      <div v-if="webSearchConfig.enabled" class="space-y-4">
        <div class="flex items-center justify-between">
          <label class="text-sm font-medium text-gray-700 dark:text-gray-300">
            {{ t("admin.settings.webSearchEmulation.providers") }}
          </label>
          <button
            type="button"
            class="btn btn-secondary btn-sm"
            @click="addWebSearchProvider"
          >
            {{ t("admin.settings.webSearchEmulation.addProvider") }}
          </button>
        </div>

        <div
          v-if="webSearchConfig.providers.length === 0"
          class="rounded-lg border border-dashed border-gray-300 p-4 text-center text-sm text-gray-400 dark:border-dark-600"
        >
          {{ t("admin.settings.webSearchEmulation.noProviders") }}
        </div>

        <div
          v-for="(provider, pIdx) in webSearchConfig.providers"
          :key="pIdx"
          class="rounded-lg border border-gray-200 dark:border-dark-600"
        >
          <div
            class="flex cursor-pointer items-center justify-between px-4 py-3"
            @click="toggleProviderExpand(pIdx)"
          >
            <div class="flex items-center gap-3">
              <svg
                class="h-4 w-4 text-gray-400 transition-transform"
                :class="{ 'rotate-90': expandedProviders[pIdx] }"
                fill="none"
                viewBox="0 0 24 24"
                stroke="currentColor"
              >
                <path
                  stroke-linecap="round"
                  stroke-linejoin="round"
                  stroke-width="2"
                  d="M9 5l7 7-7 7"
                />
              </svg>
              <Select
                v-model="provider.type"
                :options="[
                  { value: 'brave', label: 'Brave Search' },
                  { value: 'tavily', label: 'Tavily' },
                  anySearchProviderOption,
                ]"
                class="w-36"
                @click.stop
              />
              <span class="text-xs text-gray-400">
                {{ provider.quota_used ?? 0 }} /
                {{
                  provider.quota_limit != null && provider.quota_limit > 0
                    ? provider.quota_limit
                    : "∞"
                }}
              </span>
              <span
                v-if="!expandedProviders[pIdx] && provider.api_key_configured"
                class="text-xs text-green-500"
              >
                {{ t("admin.settings.webSearchEmulation.apiKeyConfigured") }}
              </span>
            </div>
            <button
              type="button"
              class="text-red-500 hover:text-red-700 text-xs"
              @click.stop="removeWebSearchProvider(pIdx)"
            >
              {{ t("admin.settings.webSearchEmulation.removeProvider") }}
            </button>
          </div>

          <div
            v-if="expandedProviders[pIdx]"
            class="space-y-3 border-t border-gray-100 px-4 pb-4 pt-3 dark:border-dark-700"
          >
            <div>
              <label class="text-xs text-gray-500">
                {{ t("admin.settings.webSearchEmulation.apiKey") }}
              </label>
              <div class="relative">
                <input
                  v-model="provider.api_key"
                  :type="apiKeyVisible[pIdx] ? 'text' : 'password'"
                  class="input w-full text-sm"
                  :class="provider.api_key || provider.api_key_configured ? 'pr-16' : ''"
                  :placeholder="
                    provider.api_key_configured
                      ? '••••••••'
                      : t(webSearchProviderAPIKeyPlaceholderKey(provider.type))
                  "
                />
                <div
                  v-if="provider.api_key || provider.api_key_configured"
                  class="absolute inset-y-0 right-0 flex items-center pr-1.5"
                >
                  <button
                    type="button"
                    class="rounded p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                    :title="
                      apiKeyVisible[pIdx]
                        ? t('admin.settings.webSearchEmulation.hideApiKey')
                        : t('admin.settings.webSearchEmulation.showApiKey')
                    "
                    @click="apiKeyVisible[pIdx] = !apiKeyVisible[pIdx]"
                  >
                    <svg
                      v-if="!apiKeyVisible[pIdx]"
                      class="h-4 w-4"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M15 12a3 3 0 11-6 0 3 3 0 016 0z"
                      />
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M2.458 12C3.732 7.943 7.523 5 12 5c4.478 0 8.268 2.943 9.542 7-1.274 4.057-5.064 7-9.542 7-4.477 0-8.268-2.943-9.542-7z"
                      />
                    </svg>
                    <svg
                      v-else
                      class="h-4 w-4"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M13.875 18.825A10.05 10.05 0 0112 19c-4.478 0-8.268-2.943-9.543-7a9.97 9.97 0 011.563-3.029m5.858.908a3 3 0 114.243 4.243M9.878 9.878l4.242 4.242M9.878 9.878L3 3m6.878 6.878L21 21"
                      />
                    </svg>
                  </button>
                  <button
                    type="button"
                    class="rounded p-1 text-gray-400 hover:text-gray-600 dark:hover:text-gray-300"
                    :class="{ 'opacity-30 cursor-not-allowed': !provider.api_key }"
                    :title="t('admin.settings.webSearchEmulation.copyApiKey')"
                    :disabled="!provider.api_key"
                    @click="copyApiKey(pIdx)"
                  >
                    <svg
                      class="h-4 w-4"
                      fill="none"
                      viewBox="0 0 24 24"
                      stroke="currentColor"
                    >
                      <path
                        stroke-linecap="round"
                        stroke-linejoin="round"
                        stroke-width="2"
                        d="M8 16H6a2 2 0 01-2-2V6a2 2 0 012-2h8a2 2 0 012 2v2m-6 12h8a2 2 0 002-2v-8a2 2 0 00-2-2h-8a2 2 0 00-2 2v8a2 2 0 002 2z"
                      />
                    </svg>
                  </button>
                </div>
              </div>
              <AnySearchAPIKeyHint :provider-type="provider.type" />
            </div>

            <div class="grid grid-cols-2 gap-3">
              <div>
                <label class="text-xs text-gray-500">
                  {{ t("admin.settings.webSearchEmulation.quotaLimit") }}
                </label>
                <input
                  v-model="provider.quota_limit"
                  type="number"
                  min="1"
                  class="input text-sm"
                  :placeholder="'∞'"
                />
                <p class="mt-0.5 text-xs text-gray-400">
                  {{ t("admin.settings.webSearchEmulation.quotaLimitHint") }}
                </p>
              </div>
              <div>
                <label class="text-xs text-gray-500">
                  {{ t("admin.settings.webSearchEmulation.subscribedAt") }}
                </label>
                <input
                  :value="formatSubscribedAt(provider.subscribed_at)"
                  type="date"
                  class="input text-sm"
                  @input="
                    provider.subscribed_at = parseSubscribedAt(
                      ($event.target as HTMLInputElement).value,
                    )
                  "
                />
                <p class="mt-0.5 text-xs text-gray-400">
                  {{ t("admin.settings.webSearchEmulation.subscribedAtHint") }}
                </p>
              </div>
            </div>

            <div class="flex items-center gap-2">
              <span class="text-xs text-gray-500">
                {{ t("admin.settings.webSearchEmulation.quotaUsage") }}:
              </span>
              <div
                v-if="provider.quota_limit != null && provider.quota_limit > 0"
                class="flex-1 rounded-full bg-gray-200 dark:bg-dark-600"
                style="height: 6px"
              >
                <div
                  class="h-full rounded-full transition-all"
                  :class="
                    quotaPercentage(provider) > 90
                      ? 'bg-red-500'
                      : quotaPercentage(provider) > 70
                        ? 'bg-yellow-500'
                        : 'bg-green-500'
                  "
                  :style="{ width: Math.min(quotaPercentage(provider), 100) + '%' }"
                />
              </div>
              <div v-else class="flex-1" />
              <span class="text-xs text-gray-500">
                {{ provider.quota_used ?? 0 }} /
                {{
                  provider.quota_limit != null && provider.quota_limit > 0
                    ? provider.quota_limit
                    : "∞"
                }}
              </span>
              <button
                v-if="(provider.quota_used ?? 0) > 0"
                type="button"
                class="text-xs text-primary-600 hover:text-primary-700"
                @click="resetWebSearchUsage(pIdx)"
              >
                {{ t("admin.settings.webSearchEmulation.resetUsage") }}
              </button>
            </div>

            <div class="flex items-end gap-3">
              <div class="flex-1">
                <label class="text-xs text-gray-500">
                  {{ t("admin.settings.webSearchEmulation.proxy") }}
                </label>
                <ProxySelector
                  v-model="provider.proxy_id"
                  :proxies="webSearchProxies"
                />
              </div>
              <button
                type="button"
                class="btn btn-secondary btn-sm whitespace-nowrap"
                @click="openTestDialog()"
              >
                {{ t("admin.settings.webSearchEmulation.test") }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>

  <div
    v-if="wsTestDialogOpen"
    class="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
    @click.self="wsTestDialogOpen = false"
  >
    <div class="mx-4 w-full max-w-lg rounded-xl bg-white p-6 shadow-xl dark:bg-dark-800">
      <h3 class="mb-4 text-lg font-semibold text-gray-900 dark:text-white">
        {{ t("admin.settings.webSearchEmulation.testResultTitle") }}
      </h3>
      <div class="flex items-center gap-2">
        <input
          v-model="wsTestQuery"
          type="text"
          class="input flex-1 text-sm"
          :placeholder="t('admin.settings.webSearchEmulation.testDefaultQuery')"
          @keyup.enter="testWebSearchProvider()"
        />
        <button
          type="button"
          class="btn btn-primary btn-sm"
          :disabled="wsTestLoading"
          @click="testWebSearchProvider()"
        >
          {{
            wsTestLoading
              ? t("admin.settings.webSearchEmulation.testing")
              : t("admin.settings.webSearchEmulation.test")
          }}
        </button>
      </div>
      <div
        v-if="wsTestResult"
        class="mt-4 max-h-80 overflow-y-auto rounded-lg bg-gray-50 p-4 dark:bg-dark-700"
      >
        <p class="mb-2 text-sm font-medium text-gray-700 dark:text-gray-300">
          {{ t("admin.settings.webSearchEmulation.testResultProvider") }}:
          {{ wsTestResult.provider }}
        </p>
        <div
          v-if="wsTestResult.results.length === 0"
          class="text-sm text-gray-400"
        >
          {{ t("admin.settings.webSearchEmulation.testNoResults") }}
        </div>
        <div
          v-for="(r, rIdx) in wsTestResult.results"
          :key="rIdx"
          class="mt-2 border-t border-gray-200 pt-2 first:mt-0 first:border-0 first:pt-0 dark:border-dark-600"
        >
          <a
            :href="r.url"
            target="_blank"
            class="text-sm font-medium text-blue-600 hover:underline dark:text-blue-400"
          >
            {{ r.title }}
          </a>
          <p class="mt-0.5 text-xs text-gray-500 dark:text-gray-400">
            {{ r.snippet }}
          </p>
        </div>
      </div>
      <div class="mt-4 flex justify-end">
        <button
          type="button"
          class="btn btn-secondary btn-sm"
          @click="wsTestDialogOpen = false"
        >
          {{ t("common.close") }}
        </button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { useI18n } from "vue-i18n";
import { adminAPI } from "@/api";
import type {
  WebSearchEmulationConfig,
  WebSearchProviderConfig,
  WebSearchTestResult,
} from "@/api/admin/settings";
import type { Proxy } from "@/types";
import Select from "@/components/common/Select.vue";
import Toggle from "@/components/common/Toggle.vue";
import ProxySelector from "@/components/common/ProxySelector.vue";
import { useClipboard } from "@/composables/useClipboard";
import { useAppStore } from "@/stores";
import { extractApiErrorMessage } from "@/utils/apiError";
import AnySearchAPIKeyHint from "./AnySearchAPIKeyHint.vue";
import {
  anySearchProviderOption,
  hasRequiredWebSearchProviderAPIKey,
  webSearchProviderAPIKeyPlaceholderKey,
} from "./anySearch";

const { t } = useI18n();
const appStore = useAppStore();
const { copyToClipboard } = useClipboard();

/** 新增 Web Search provider 的默认配额上限。 */
const DEFAULT_WEB_SEARCH_QUOTA_LIMIT = 1000;

const webSearchProxies = ref<Proxy[]>([]);
const webSearchConfig = reactive<WebSearchEmulationConfig>({
  enabled: false,
  providers: [],
});
const expandedProviders = reactive<Record<number, boolean>>({});
const apiKeyVisible = reactive<Record<number, boolean>>({});
const wsTestQuery = ref("");
const wsTestLoading = ref(false);
const wsTestResult = ref<WebSearchTestResult | null>(null);
const wsTestDialogOpen = ref(false);
let webSearchConfigLoaded = false;
let webSearchConfigLoadPromise: Promise<void> | null = null;

/** 打开搜索测试弹窗并清理上一次结果。 */
function openTestDialog(): void {
  wsTestResult.value = null;
  wsTestDialogOpen.value = true;
}

/**
 * 切换指定 provider 的展开状态。
 *
 * @param idx provider 在当前配置列表中的索引。
 */
function toggleProviderExpand(idx: number): void {
  expandedProviders[idx] = !expandedProviders[idx];
}

/**
 * 删除 provider 并重建索引状态，避免后续行复用旧展开和显隐状态。
 *
 * @param idx 要删除的 provider 索引。
 */
function removeWebSearchProvider(idx: number): void {
  webSearchConfig.providers.splice(idx, 1);
  const newExpanded: Record<number, boolean> = {};
  const newVisible: Record<number, boolean> = {};
  for (let i = 0; i < webSearchConfig.providers.length; i++) {
    const oldIdx = i >= idx ? i + 1 : i;
    newExpanded[i] = expandedProviders[oldIdx] ?? false;
    newVisible[i] = apiKeyVisible[oldIdx] ?? false;
  }
  Object.keys(expandedProviders).forEach((key) => delete expandedProviders[Number(key)]);
  Object.keys(apiKeyVisible).forEach((key) => delete apiKeyVisible[Number(key)]);
  Object.assign(expandedProviders, newExpanded);
  Object.assign(apiKeyVisible, newVisible);
}

/** 新增一个默认展开的 Brave provider 配置。 */
function addWebSearchProvider(): void {
  const idx = webSearchConfig.providers.length;
  webSearchConfig.providers.push({
    type: "brave",
    api_key: "",
    api_key_configured: false,
    quota_limit: DEFAULT_WEB_SEARCH_QUOTA_LIMIT,
    subscribed_at: null,
    proxy_id: null,
    expires_at: null,
  });
  expandedProviders[idx] = true;
}

/**
 * 将 Unix 秒时间戳格式化为 date input 使用的 UTC 日期。
 *
 * @param ts Unix 秒时间戳。
 * @return `YYYY-MM-DD` 日期字符串，空值返回空字符串。
 */
function formatSubscribedAt(ts: number | null): string {
  if (!ts) return "";
  // 使用 UTC 是为了避免反复编辑日期时受本地时区影响产生前后一天漂移。
  const date = new Date(ts * 1000);
  const year = date.getUTCFullYear();
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  const day = String(date.getUTCDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

/**
 * 将 date input 的 UTC 日期转换为 Unix 秒时间戳。
 *
 * @param dateStr `YYYY-MM-DD` 日期字符串。
 * @return Unix 秒时间戳，空值返回 null。
 */
function parseSubscribedAt(dateStr: string): number | null {
  if (!dateStr) return null;
  return Math.floor(new Date(`${dateStr}T00:00:00Z`).getTime() / 1000);
}

/**
 * 计算 provider 当前配额使用百分比。
 *
 * @param provider Web Search provider 配置。
 * @return 当前使用百分比，无有效上限时返回 0。
 */
function quotaPercentage(provider: WebSearchProviderConfig): number {
  if (!provider.quota_limit || provider.quota_limit <= 0) return 0;
  return ((provider.quota_used ?? 0) / provider.quota_limit) * 100;
}

/**
 * 重置指定 provider 的远端用量计数。
 *
 * @param idx provider 在当前配置列表中的索引。
 */
async function resetWebSearchUsage(idx: number): Promise<void> {
  const provider = webSearchConfig.providers[idx];
  if (!provider) return;
  if (!confirm(t("admin.settings.webSearchEmulation.resetUsageConfirm"))) return;
  try {
    await adminAPI.settings.resetWebSearchUsage({
      provider_type: provider.type,
    });
    provider.quota_used = 0;
    appStore.showSuccess(t("admin.settings.webSearchEmulation.resetUsageSuccess"));
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  }
}

/**
 * 复制 provider 当前新输入的 API Key。
 *
 * @param idx provider 在当前配置列表中的索引。
 */
async function copyApiKey(idx: number): Promise<void> {
  const key = webSearchConfig.providers[idx]?.api_key;
  if (!key) {
    appStore.showError(t("admin.settings.webSearchEmulation.apiKeyPlaceholder"));
    return;
  }
  await copyToClipboard(key, t("admin.settings.webSearchEmulation.copied"));
}

/** 使用当前配置的 Web Search manager 发起一次测试搜索。 */
async function testWebSearchProvider(): Promise<void> {
  wsTestLoading.value = true;
  wsTestResult.value = null;
  try {
    const query =
      wsTestQuery.value.trim() ||
      t("admin.settings.webSearchEmulation.testDefaultQuery");
    wsTestResult.value = await adminAPI.settings.testWebSearchEmulation(query);
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
  } finally {
    wsTestLoading.value = false;
  }
}

/**
 * 从后端加载 Web Search Emulation 配置和可选代理列表。
 *
 * @return 加载完成后的 Promise。
 */
async function runLoad(): Promise<void> {
  try {
    const [resp, proxiesResp] = await Promise.all([
      adminAPI.settings.getWebSearchEmulationConfig(),
      adminAPI.proxies.list().catch(() => ({ items: [] as Proxy[] })),
    ]);
    if (resp) {
      webSearchConfig.enabled = resp.enabled || false;
      webSearchConfig.providers = resp.providers || [];
    }
    webSearchProxies.value = proxiesResp.items || [];
    webSearchConfigLoaded = true;
  } catch (err: unknown) {
    const status = (err as { status?: number })?.status;
    if (status === 404) {
      webSearchConfigLoaded = true;
      return;
    }
    if (status !== undefined) {
      appStore.showError(extractApiErrorMessage(err, t("common.error")));
    }
  }
}

/**
 * 从后端加载 Web Search Emulation 配置和可选代理列表。
 *
 * @return 加载完成后的 Promise。
 */
async function load(): Promise<void> {
  if (webSearchConfigLoadPromise) return webSearchConfigLoadPromise;
  const promise = runLoad();
  webSearchConfigLoadPromise = promise;
  try {
    await promise;
  } finally {
    if (webSearchConfigLoadPromise === promise) {
      webSearchConfigLoadPromise = null;
    }
  }
}

/**
 * 确保保存前已经拿到后端现有配置，避免用默认空配置覆盖真实配置。
 *
 * @return 已成功完成初始加载时返回 true。
 */
async function ensureWebSearchConfigLoaded(): Promise<boolean> {
  if (webSearchConfigLoaded) return true;
  if (webSearchConfigLoadPromise) {
    await webSearchConfigLoadPromise;
  }
  if (webSearchConfigLoaded) return true;
  await load();
  return webSearchConfigLoaded;
}

/**
 * 校验并保存 Web Search Emulation 配置。
 *
 * @return 保存成功返回 true；校验失败或保存失败返回 false。
 */
async function save(): Promise<boolean> {
  try {
    if (!(await ensureWebSearchConfigLoaded())) {
      appStore.showError(t("common.error"));
      return false;
    }
    for (const provider of webSearchConfig.providers) {
      if (
        webSearchConfig.enabled &&
        !hasRequiredWebSearchProviderAPIKey(provider)
      ) {
        appStore.showError(
          t("admin.settings.webSearchEmulation.apiKeyRequired", {
            provider: provider.type,
          }),
        );
        return false;
      }
      const raw = provider.quota_limit;
      if (raw != null && Number(raw) !== 0 && Number(raw) < 1) {
        appStore.showError(
          t("admin.settings.webSearchEmulation.quotaLimitMustBePositive"),
        );
        return false;
      }
    }
    const providers = webSearchConfig.providers.map((provider) => ({
      ...provider,
      quota_limit:
        Number(provider.quota_limit) > 0 ? Number(provider.quota_limit) : null,
    }));
    await adminAPI.settings.updateWebSearchEmulationConfig({
      enabled: webSearchConfig.enabled,
      providers,
    });
    return true;
  } catch (err: unknown) {
    appStore.showError(extractApiErrorMessage(err, t("common.error")));
    return false;
  }
}

onMounted(() => {
  void load();
});

defineExpose({
  load,
  save,
});
</script>
