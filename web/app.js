const form = document.querySelector("#checkoutForm");
const submitButton = document.querySelector("#submitButton");
const paypalSubmitButton = document.querySelector("#paypalSubmitButton");
const paymentSubmitButtons = [submitButton, paypalSubmitButton].filter(Boolean);
const output = document.querySelector("#output");
const healthBadge = document.querySelector("#healthBadge");
const latency = document.querySelector("#latency");
const statusText = document.querySelector("#statusText");
const sessionText = document.querySelector("#sessionText");
const hostText = document.querySelector("#hostText");
const finalTransactionText = document.querySelector("#finalTransactionText");
const finalSettlementText = document.querySelector("#finalSettlementText");
const checkoutUrlLink = document.querySelector("#checkoutUrlLink");
const copyCheckoutLinkButton = document.querySelector("#copyCheckoutLinkButton");
const openInIncognitoBtn = document.querySelector("#openInIncognitoBtn");
const openIncognitoBtn = document.querySelector("#openIncognitoBtn");
const fetchSessionBtn = document.querySelector("#fetchSessionBtn");
const openPaymentShortcutBtn = document.querySelector("#openPaymentShortcutBtn");
const exportSub2APIBtn = document.querySelector("#exportSub2APIBtn");
const closeIncognitoBtn = document.querySelector("#closeIncognitoBtn");
const incognitoDiagBadge = document.querySelector("#incognitoDiagBadge");
const incognitoDiagCDP = document.querySelector("#incognitoDiagCDP");
const incognitoDiagTargets = document.querySelector("#incognitoDiagTargets");
const incognitoDiagReadyState = document.querySelector("#incognitoDiagReadyState");
const incognitoDiagLoad = document.querySelector("#incognitoDiagLoad");
const incognitoDiagMessage = document.querySelector("#incognitoDiagMessage");
const incognitoDiagRefreshBtn = document.querySelector("#incognitoDiagRefreshBtn");
const incognitoDiagOpenBtn = document.querySelector("#incognitoDiagOpenBtn");
const incognitoDiagSlow = document.querySelector("#incognitoDiagSlow");
const incognitoDiagOutput = document.querySelector("#incognitoDiagOutput");
const alertBox = document.querySelector("#alertBox");
const alertTitle = document.querySelector("#alertTitle");
const alertBadge = document.querySelector("#alertBadge");
const alertMessage = document.querySelector("#alertMessage");
const alertHint = document.querySelector("#alertHint");
const alertSteps = document.querySelector("#alertSteps");
const alertStepsList = document.querySelector("#alertStepsList");
const stepFlowItems = Array.from(document.querySelectorAll(".step-flow-item"));
const stepInputText = document.querySelector("#stepInputText");
const stepTokenText = document.querySelector("#stepTokenText");
const stepCheckoutText = document.querySelector("#stepCheckoutText");
const stepCookieText = document.querySelector("#stepCookieText");
const stepLinkText = document.querySelector("#stepLinkText");

const wechatGroupCard = document.querySelector("#wechatGroupCard");
const wechatGroupToggleBtn = document.querySelector("#wechatGroupToggleBtn");
const wechatGroupPreviewBtn = document.querySelector("#wechatGroupPreviewBtn");
const wechatGroupCloseBtn = document.querySelector("#wechatGroupCloseBtn");
const wechatGroupModal = document.querySelector("#wechatGroupModal");
const wechatGroupBackdrop = document.querySelector("#wechatGroupBackdrop");
const wechatGroupModalCloseBtn = document.querySelector("#wechatGroupModalCloseBtn");

let latestCheckoutURL = "";
let latestOpenedCheckoutURL = "";
let latestOpenedCheckoutTargetID = "";
let activePaymentLabel = "Popay";
let incognitoWindowOpened = false;
let loginClickReady = false;
let autoFetchAndGenerateRunning = false;
let activeAutomationTraceID = "";
let plusSubscribeWatcherTimer = null;
let plusSubscribeWatcherExpiresAt = 0;
let plusSubscribeProbeInFlight = false;
let plusSubscribeAutoTriggeredSignature = "";
let plusSubscribeAutoTriggerLastAttemptAt = 0;
let plusSubscribeWatcherReason = "watch";
let plusSubscribeProbeConsecutiveMisses = 0;
let plusSubscribeProbeLastSignature = "";
const clientPerfState = {
  longTasks: [],
  slowInteractions: [],
  errors: [],
};

function createAutomationTraceID(prefix) {
  var safePrefix = String(prefix || "flow").replace(/[^a-z0-9_-]+/gi, "").toLowerCase() || "flow";
  return safePrefix + "-" + Date.now().toString(36) + "-" + Math.random().toString(36).slice(2, 8);
}

function ensureAutomationTraceID(prefix) {
  if (!activeAutomationTraceID) {
    activeAutomationTraceID = createAutomationTraceID(prefix);
  }
  return activeAutomationTraceID;
}

function safePerfURL(rawURL) {
  try {
    const parsed = new URL(String(rawURL || ""), window.location.href);
    const params = [];
    parsed.searchParams.forEach(function (_value, key) {
      if (!params.includes(key)) params.push(key);
    });
    const query = params.length ? "?" + params.map(function (key) { return encodeURIComponent(key) + "=***"; }).join("&") : "";
    var hash = parsed.hash || "";
    if (hash.includes("?")) {
      const parts = hash.split("?");
      const route = parts.shift();
      const hashParams = new URLSearchParams(parts.join("?"));
      const hashNames = [];
      hashParams.forEach(function (_value, key) {
        if (!hashNames.includes(key)) hashNames.push(key);
      });
      hash = route + (hashNames.length ? "?" + hashNames.map(function (key) { return encodeURIComponent(key) + "=***"; }).join("&") : "");
    }
    return parsed.origin + parsed.pathname + query + hash;
  } catch (_error) {
    return String(rawURL || "").split("?")[0].slice(0, 180);
  }
}

function initClientPerformanceObservers() {
  window.addEventListener("error", function (event) {
    clientPerfState.errors.push({
      type: "error",
      message: String(event.message || "script error").slice(0, 240),
      source: safePerfURL(event.filename || ""),
      line: event.lineno || 0,
      column: event.colno || 0,
    });
  });
  window.addEventListener("unhandledrejection", function (event) {
    clientPerfState.errors.push({
      type: "unhandledrejection",
      message: String(event.reason?.message || event.reason || "promise rejection").slice(0, 240),
    });
  });
  if (!window.PerformanceObserver) return;
  try {
    if (PerformanceObserver.supportedEntryTypes?.includes("longtask")) {
      new PerformanceObserver(function (list) {
        list.getEntries().slice(-20).forEach(function (entry) {
          clientPerfState.longTasks.push({
            name: entry.name,
            start_ms: Math.round(entry.startTime),
            duration_ms: Math.round(entry.duration),
          });
        });
        clientPerfState.longTasks = clientPerfState.longTasks.slice(-20);
      }).observe({ entryTypes: ["longtask"] });
    }
  } catch (_error) {}
  try {
    if (PerformanceObserver.supportedEntryTypes?.includes("event")) {
      new PerformanceObserver(function (list) {
        list.getEntries().slice(-20).forEach(function (entry) {
          if (entry.duration >= 40) {
            clientPerfState.slowInteractions.push({
              name: entry.name,
              start_ms: Math.round(entry.startTime),
              duration_ms: Math.round(entry.duration),
            });
          }
        });
        clientPerfState.slowInteractions = clientPerfState.slowInteractions.slice(-20);
      }).observe({ type: "event", buffered: true, durationThreshold: 40 });
    }
  } catch (_error) {}
}

function buildClientPerformancePayload() {
  const nav = performance.getEntriesByType("navigation")[0];
  const resources = performance.getEntriesByType("resource").map(function (entry) {
    return {
      url: safePerfURL(entry.name),
      initiator_type: entry.initiatorType,
      start_ms: Math.round(entry.startTime),
      duration_ms: Math.round(entry.duration),
      transfer_size: entry.transferSize || 0,
      encoded_size: entry.encodedBodySize || 0,
      decoded_size: entry.decodedBodySize || 0,
      response_end_ms: Math.round(entry.responseEnd || 0),
    };
  }).slice(-80);
  return {
    page_url: safePerfURL(window.location.href),
    recorded_at: new Date().toISOString(),
    nav: nav ? {
      duration_ms: Math.round(nav.duration),
      dom_content_loaded_ms: Math.round(nav.domContentLoadedEventEnd),
      load_event_ms: Math.round(nav.loadEventEnd),
      response_start_ms: Math.round(nav.responseStart),
      response_end_ms: Math.round(nav.responseEnd),
      transfer_size: nav.transferSize || 0,
      encoded_size: nav.encodedBodySize || 0,
      decoded_size: nav.decodedBodySize || 0,
    } : null,
    resources: resources,
    resource_totals: {
      count: resources.length,
      transfer_size: resources.reduce(function (sum, item) { return sum + item.transfer_size; }, 0),
      decoded_size: resources.reduce(function (sum, item) { return sum + item.decoded_size; }, 0),
    },
    long_tasks: clientPerfState.longTasks.slice(-20),
    slow_interactions: clientPerfState.slowInteractions.slice(-20),
    errors: clientPerfState.errors.slice(-20),
  };
}

function sendClientPerformanceLog() {
  try {
    const body = JSON.stringify(buildClientPerformancePayload());
    if (navigator.sendBeacon) {
      const blob = new Blob([body], { type: "application/json" });
      if (navigator.sendBeacon("/api/perf/client", blob)) return;
    }
    fetch("/api/perf/client", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: body,
      keepalive: true,
    }).catch(function () {});
  } catch (_error) {}
}

initClientPerformanceObservers();
window.addEventListener("load", function () {
  window.setTimeout(sendClientPerformanceLog, 2500);
});

const fields = {
  token: document.querySelector("#token"),
  entryPoint: document.querySelector("#entryPoint"),
  planName: document.querySelector("#planName"),
  country: document.querySelector("#country"),
  currency: document.querySelector("#currency"),
  promoCampaignId: document.querySelector("#promoCampaignId"),
  couponFromQuery: document.querySelector("#couponFromQuery"),
  checkoutUIMode: document.querySelector("#checkoutUIMode"),
  checkoutCookie: document.querySelector("#checkoutCookie"),
  checkoutUserAgent: document.querySelector("#checkoutUserAgent"),
  customerEmail: document.querySelector("#customerEmail"),
};

function setText(node, value) {
  if (node) {
    node.textContent = value;
  }
}

const workflowVoiceSteps = {
  openIncognitoBtn: { order: 1, label: "打开无痕窗口" },
  luckmailDeviceLoginGuideBtn: { order: 2, label: "设备登录引导" },
  fetchSessionBtn: { order: 3, label: "获取 Session JSON" },
  autoFillCheckoutBtn: { order: 4, label: "自动填地址" },
  openPaymentShortcutBtn: { order: 5, label: "打开支付页" },
  gopayLinkBtn: { order: 6, label: "GoPay 绑定" },
  luckmailTokenCodeBtn: { order: 7, label: "LuckMail 接码" },
  browserUseRunBtn: { order: 8, label: "Browser Use 分析" },
  exportSub2APIBtn: { order: 9, label: "导出 sub2api 凭证" },
  closeIncognitoBtn: { order: 10, label: "关闭窗口" },
};

const workflowVoiceStateMessages = {
  running: "开始执行",
  done: "已完成",
  error: "执行失败",
};

const workflowVoiceStepMessages = {
  openIncognitoBtn: {
    running: "第一步，正在打开无痕窗口。",
    done: "第一步完成，无痕窗口已打开。下一步进行设备登录引导。",
    error: "第一步失败，无痕窗口没有成功打开，请检查 Chrome 或本地 CDP 服务。",
  },
  luckmailDeviceLoginGuideBtn: {
    running: "第二步，正在执行设备登录引导，系统会准备邮箱并打开登录页面。",
    done: "第二步完成，设备登录引导已结束。下一步获取 Session JSON。",
    error: "第二步失败，设备登录引导没有完成，请查看 LuckMail 状态信息。",
  },
  fetchSessionBtn: {
    running: "第三步，正在获取 Session JSON。",
    done: "第三步完成，Session JSON 已获取。下一步自动填地址。",
    error: "第三步失败，Session JSON 获取失败，请确认无痕窗口已登录。",
  },
  autoFillCheckoutBtn: {
    running: "第四步，正在自动填写地址和结账信息。",
    done: "第四步完成，地址信息已填写。下一步打开支付页。",
    error: "第四步失败，自动填写地址没有完成，请检查页面状态。",
  },
  openPaymentShortcutBtn: {
    running: "第五步，正在打开支付页并优先选择 GoPay。",
    done: "第五步完成，支付页已打开。下一步执行 GoPay 绑定。",
    error: "第五步失败，支付页没有成功打开，请重新生成支付链接。",
  },
  gopayLinkBtn: {
    running: "第六步，正在执行 GoPay 绑定和支付辅助。",
    done: "第六步完成，GoPay 流程已完成。",
    error: "第六步失败，GoPay 流程出现异常，请查看 GoPay 实时状态。",
  },
  luckmailTokenCodeBtn: {
    running: "第七步，正在通过 LuckMail 接收验证码。",
    done: "第七步完成，LuckMail 验证码已处理。",
    error: "第七步失败，LuckMail 未能完成接码，请检查 Token 或邮箱状态。",
  },
  browserUseRunBtn: {
    running: "第八步，正在执行 Browser Use 分析。",
    done: "第八步完成，Browser Use 分析已结束。",
    error: "第八步失败，Browser Use 分析没有完成，请查看分析输出。",
  },
  exportSub2APIBtn: {
    running: "第九步，正在导出 sub2api 凭证。",
    done: "第九步完成，sub2api 凭证已导出。",
    error: "第九步失败，sub2api 凭证导出失败，请检查登录状态。",
  },
  closeIncognitoBtn: {
    running: "第十步，正在关闭无痕窗口。",
    done: "第十步完成，无痕窗口已关闭，当前流程收尾完成。",
    error: "第十步失败，无痕窗口没有关闭，请稍后重试或手动关闭。",
  },
};

function announceWorkflowButtonState(button, state) {
  var step = workflowVoiceSteps[button?.id || ""];
  var stateText = workflowVoiceStateMessages[state || ""];
  if (!step || !stateText) return;
  if (button.dataset.voiceState === state) return;
  button.dataset.voiceState = state;
  try {
    if (typeof voicePromptEnabled !== "function" || !voicePromptEnabled()) return;
  } catch (_err) {
    return;
  }
  if (typeof announceLuckMailScene !== "function") return;
  var message = workflowVoiceStepMessages[button.id]?.[state] || ("推荐流程第 " + step.order + " 步，" + step.label + "，" + stateText);
  announceLuckMailScene("workflow_" + button.id + "_" + state, message, true);
}

function setWorkflowButtonState(button, state) {
  if (!button) return;
  var nextState = state || "idle";
  if (nextState === "idle") {
    delete button.dataset.flowState;
    delete button.dataset.voiceState;
    button.removeAttribute("aria-busy");
    return;
  }
  button.dataset.flowState = nextState;
  button.setAttribute("aria-busy", nextState === "running" ? "true" : "false");
  announceWorkflowButtonState(button, nextState);
}

window.setWorkflowButtonState = setWorkflowButtonState;

function setWechatGroupCollapsed(collapsed) {
  if (!wechatGroupCard || !wechatGroupToggleBtn) {
    return;
  }
  wechatGroupCard.classList.toggle("is-collapsed", collapsed);
  wechatGroupCard.classList.toggle("is-expanded", !collapsed);
  wechatGroupToggleBtn.setAttribute("aria-expanded", collapsed ? "false" : "true");
}

function openWechatGroupModal() {
  if (!wechatGroupModal) {
    return;
  }
  wechatGroupModal.hidden = false;
  document.body.style.overflow = "hidden";
}

function closeWechatGroupModal() {
  if (!wechatGroupModal) {
    return;
  }
  wechatGroupModal.hidden = true;
  document.body.style.overflow = "";
}

function bindWechatGroupCard() {
  if (wechatGroupToggleBtn) {
    wechatGroupToggleBtn.addEventListener("click", function () {
      var collapsed = wechatGroupCard?.classList.contains("is-collapsed");
      setWechatGroupCollapsed(!collapsed);
    });
  }

  if (wechatGroupPreviewBtn) {
    wechatGroupPreviewBtn.addEventListener("click", function () {
      openWechatGroupModal();
    });
  }

  if (wechatGroupCloseBtn) {
    wechatGroupCloseBtn.addEventListener("click", function () {
      if (wechatGroupCard) {
        wechatGroupCard.classList.add("is-hidden");
      }
      closeWechatGroupModal();
    });
  }

  if (wechatGroupBackdrop) {
    wechatGroupBackdrop.addEventListener("click", function () {
      closeWechatGroupModal();
    });
  }

  if (wechatGroupModalCloseBtn) {
    wechatGroupModalCloseBtn.addEventListener("click", function () {
      closeWechatGroupModal();
    });
  }

  document.addEventListener("keydown", function (event) {
    if (event.key === "Escape") {
      closeWechatGroupModal();
    }
  });
}

function extractSessionAccessToken(rawValue) {
  const value = (rawValue || "").trim();
  if (!value) {
    return "";
  }
  if (value.startsWith("eyJ")) {
    return value;
  }
  try {
    const parsed = JSON.parse(value);
    const token = parsed?.accessToken || parsed?.access_token;
    if (typeof token === "string" && token.trim()) {
      return token.trim();
    }
  } catch (error) {
    const markers = ['"accessToken":"', '"accessToken": "', '"access_token":"', '"access_token": "'];
    for (const marker of markers) {
      const start = value.indexOf(marker);
      if (start >= 0) {
        const valueStart = start + marker.length;
        const end = value.indexOf('"', valueStart);
        if (end > valueStart) {
          return value.slice(valueStart, end).trim();
        }
      }
    }
  }
  return "";
}

function extractAccessToken(rawValue) {
  const value = (rawValue || "").trim();
  if (!value) {
    return "";
  }
  const token = extractSessionAccessToken(value);
  if (token) {
    return token;
  }
  return value;
}

function parseJSONText(rawValue) {
  try {
    return JSON.parse(String(rawValue || ""));
  } catch (_error) {
    return null;
  }
}

function decodeBase64URL(value) {
  var input = String(value || "").replace(/-/g, "+").replace(/_/g, "/");
  while (input.length % 4) input += "=";
  return decodeURIComponent(Array.prototype.map.call(atob(input), function (char) {
    return "%" + ("00" + char.charCodeAt(0).toString(16)).slice(-2);
  }).join(""));
}

function decodeJWTPayload(token) {
  var parts = String(token || "").split(".");
  if (parts.length < 2) return {};
  return parseJSONText(decodeBase64URL(parts[1])) || {};
}

function isoFromUnixSeconds(value) {
  var number = Number(value || 0);
  if (!Number.isFinite(number) || number <= 0) return "";
  return new Date(number * 1000).toISOString();
}

function sanitizeFilenamePart(value) {
  return String(value || "openai").replace(/[^a-zA-Z0-9._-]+/g, "_").replace(/^_+|_+$/g, "").slice(0, 80) || "openai";
}

function downloadJSONFile(filename, payload) {
  var blob = new Blob([JSON.stringify(payload, null, 2)], { type: "application/json" });
  var url = URL.createObjectURL(blob);
  var link = document.createElement("a");
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}

function buildSub2APIExport(sessionTextValue) {
  var parsed = parseJSONText(sessionTextValue) || {};
  var accessToken = extractAccessToken(sessionTextValue);
  var idToken = parsed.idToken || parsed.id_token || "";
  var refreshToken = parsed.refreshToken || parsed.refresh_token || "";
  var accessPayload = decodeJWTPayload(accessToken);
  var idPayload = decodeJWTPayload(idToken);
  var auth = accessPayload["https://api.openai.com/auth"] || idPayload["https://api.openai.com/auth"] || {};
  var profile = accessPayload["https://api.openai.com/profile"] || {};
  var email = parsed?.user?.email || parsed.email || profile.email || idPayload.email || "";
  var accountName = email || parsed?.user?.name || "openai";
  var exportedAt = new Date().toISOString();
  var datePart = exportedAt.slice(0, 10);
  var credentials = {
    access_token: accessToken,
    expires_at: isoFromUnixSeconds(accessPayload.exp) || parsed.expires || parsed.expires_at,
    refresh_token: refreshToken,
    id_token: idToken,
    email: email,
    chatgpt_account_id: auth.chatgpt_account_id || "",
    chatgpt_user_id: auth.chatgpt_user_id || accessPayload["https://api.openai.com/auth"]?.user_id || "",
    plan_type: auth.chatgpt_plan_type || "",
    subscription_expires_at: auth.chatgpt_subscription_active_until || ""
  };
  return {
    filename: sanitizeFilenamePart(accountName) + "_sub2api_" + datePart + ".json",
    payload: {
      exported_at: exportedAt,
      proxies: [],
      accounts: [{
        name: accountName,
        platform: "openai",
        type: "oauth",
        credentials: credentials,
        concurrency: 0,
        priority: 0
      }],
      type: "sub2api-data",
      version: 1
    },
    missing: Object.keys(credentials).filter(function (key) { return !credentials[key]; })
  };
}

async function exportSub2APICredentials() {
  if (!exportSub2APIBtn) return;
  if (!window.confirm("即将导出 sub2api 登录凭证 JSON。该文件包含可用登录凭证，请只在本人账号和本机环境使用，并妥善保存。是否继续？")) return;
  var originalText = exportSub2APIBtn.textContent;
  exportSub2APIBtn.disabled = true;
  setWorkflowButtonState(exportSub2APIBtn, "running");
  if (fetchSessionBtn) fetchSessionBtn.disabled = true;
  setText(exportSub2APIBtn, "读取 Session...");
  try {
    var sessionJSON = await fetchLatestSessionJSON();
    setText(exportSub2APIBtn, "生成 JSON...");
    var result = buildSub2APIExport(sessionJSON);
    if (!result.payload.accounts[0].credentials.access_token) {
      throw new Error("未提取到 access_token，无法生成 sub2api 凭证文件");
    }
    downloadJSONFile(result.filename, result.payload);
    if (monitorBadge) { monitorBadge.textContent = "凭证已导出"; monitorBadge.className = "badge"; }
    setWorkflowButtonState(exportSub2APIBtn, "done");
    showAutoFillNotice("sub2api 凭证已导出", "成功", "已按参考文件格式生成 JSON 下载文件。" + (result.missing.length ? " 未获取字段：" + result.missing.join(", ") : ""), "请妥善保存该文件，不要上传到公开仓库或聊天窗口。", "");
  } catch (error) {
    if (monitorBadge) { monitorBadge.textContent = "导出失败"; monitorBadge.className = "badge error"; }
    setWorkflowButtonState(exportSub2APIBtn, "error");
    showAutoFillNotice("sub2api 导出失败", "错误", error.message || "导出 sub2api 凭证失败", "请确认无痕窗口已登录 ChatGPT 后重试。", "error");
  } finally {
    sessionMonitorAbortController = null;
    exportSub2APIBtn.disabled = false;
    if (fetchSessionBtn) fetchSessionBtn.disabled = false;
    setText(exportSub2APIBtn, originalText || "导出 sub2api 凭证");
  }
}

function randomGmail() {
  const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789";
  const suffix = Array.from({ length: 10 }, () => alphabet[Math.floor(Math.random() * alphabet.length)]).join("");
  return `gpt.billing.${suffix}@gmail.com`;
}

async function checkHealth() {
  try {
    const response = await fetch("/api/health");
    if (!response.ok) {
      throw new Error("health check failed");
    }
    setText(healthBadge, "在线");
    healthBadge?.classList.remove("error");
  } catch (error) {
    setText(healthBadge, "离线");
    healthBadge?.classList.add("error");
  }
}

function buildStartPayload(options) {
  options = options || {};
  var billingDetails = options.billingDetails || {};
  var paymentMethod = String(options.paymentMethod || "gopay").trim().toLowerCase();
  var country = String(billingDetails.country || fields.country.value || "").trim().toUpperCase();
  var currency = String(billingDetails.currency || fields.currency.value || "").trim().toUpperCase();
  fields.token.value = extractAccessToken(fields.token.value);
  fields.customerEmail.value = randomGmail();
  return {
    token: fields.token.value.trim(),
    entry_point: fields.entryPoint.value.trim(),
    plan_name: fields.planName.value.trim(),
    payment_method: paymentMethod,
    billing_details: {
      country: country,
      currency: currency,
    },
    promo_campaign: {
      promo_campaign_id: fields.promoCampaignId.value.trim(),
      is_coupon_from_query_param: fields.couponFromQuery.checked,
    },
    checkout_ui_mode: fields.checkoutUIMode.value,
    checkout_session: {
      cookie: fields.checkoutCookie.value.trim(),
      user_agent: fields.checkoutUserAgent.value.trim(),
    },
    customer_email: fields.customerEmail.value.trim(),
    trace_id: ensureAutomationTraceID("checkout"),
  };
}

function buildRequestHeaders(extraHeaders) {
  var headers = { "Content-Type": "application/json" };
  var email = (fields.customerEmail?.value || "").trim();
  if (email) {
    headers["X-Account-Email"] = email;
  }
  if (extraHeaders && typeof extraHeaders === "object") {
    Object.assign(headers, extraHeaders);
  }
  return headers;
}

async function fetchJSONWithTimeout(url, options, timeoutMS) {
  var controller = new AbortController();
  var timer = window.setTimeout(function () { controller.abort(); }, timeoutMS || 30000);
  try {
    var response = await fetch(url, Object.assign({}, options || {}, { signal: controller.signal }));
    var data = await response.json();
    return { response: response, data: data };
  } catch (error) {
    if (error?.name === "AbortError") {
      throw new Error("请求超时，请检查本机服务或 LuckMail 网络连接");
    }
    throw error;
  } finally {
    window.clearTimeout(timer);
  }
}

async function postJSON(url, payload) {
  const response = await fetch(url, {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify(payload),
  });
  const data = await response.json();
  return { response, data };
}

async function copyText(text) {
  if (!text) {
    return false;
  }
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch (error) {
    const fallback = document.createElement("textarea");
    fallback.value = text;
    fallback.setAttribute("readonly", "");
    fallback.style.position = "absolute";
    fallback.style.left = "-9999px";
    document.body.append(fallback);
    fallback.select();
    const copied = document.execCommand("copy");
    fallback.remove();
    return copied;
  }
}

async function copyCurrentCheckoutURL() {
  if (!latestCheckoutURL || !copyCheckoutLinkButton) {
    return;
  }
  copyCheckoutLinkButton.disabled = true;
  const copied = await copyText(latestCheckoutURL);
  setText(copyCheckoutLinkButton, copied ? "已复制" : "复制失败");
  window.setTimeout(() => {
    setText(copyCheckoutLinkButton, "复制链接");
    copyCheckoutLinkButton.disabled = false;
  }, 1400);
}

/**
 * 调用后端 API，在系统 Chrome 无痕窗口中打开指定 URL
 * @param {string} url - 要打开的 URL
 * @param {boolean} newWindow - 是否创建新的无痕窗口（仅首次为 true）
 * @returns {Promise<boolean>} 是否成功
 */
async function openIncognito(url, newWindow) {
  try {
    const response = await fetch("/api/incognito/open", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ url: url, new_window: newWindow }),
    });
    const data = await response.json();
    if (response.ok && data.ok) {
      incognitoWindowOpened = true;
      loginClickReady = false;
      startPlusSubscribeSessionTriggerWatcher("incognito_opened");
      return true;
    }
    console.error("incognito open failed:", data);
    return false;
  } catch (error) {
    console.error("incognito open error:", error);
    return false;
  }
}

async function closeIncognitoWindow() {
  if (!closeIncognitoBtn) return;
  if (luckMailCodePollingActive) {
    setWorkflowButtonState(closeIncognitoBtn, "error");
    showAutoFillNotice("正在等待验证码", "请稍等", "LuckMail 正在等待验证码邮件返回，暂时不要关闭无痕窗口或刷新页面。", "等待完成后再关闭窗口。", "warning");
    announceLuckMailScene("close_blocked_during_code_polling", "正在等待验证码，请先不要关闭窗口");
    return false;
  }
  var originalText = closeIncognitoBtn.textContent || "关闭窗口";
  closeIncognitoBtn.disabled = true;
  setWorkflowButtonState(closeIncognitoBtn, "running");
  setText(closeIncognitoBtn, "关闭中...");
  try {
    const response = await fetch("/api/incognito/close", {
      method: "POST",
      headers: buildRequestHeaders(),
    });
    const data = await response.json();
    if (!response.ok || !data.ok) {
      throw new Error(data.error || "关闭无痕窗口失败");
    }
    incognitoWindowOpened = false;
    loginClickReady = false;
    latestOpenedCheckoutURL = "";
    stopPlusSubscribeSessionTriggerWatcher();
    setWorkflowButtonState(closeIncognitoBtn, "done");
    showAutoFillNotice("无痕窗口已关闭", "完成", data.stage === "incognito_close_no_targets" ? "没有发现需要关闭的无痕页面，流程已完成收尾。" : "已关闭本工具管理的无痕窗口/支付页面。", "", "success");
    return true;
  } catch (error) {
    setWorkflowButtonState(closeIncognitoBtn, "error");
    showAutoFillNotice("关闭窗口失败", "错误", error.message || "关闭无痕窗口失败", "可手动关闭 Chrome 无痕窗口后继续。", "error");
    return false;
  } finally {
    closeIncognitoBtn.disabled = false;
    setText(closeIncognitoBtn, originalText);
  }
}

function formatDiagMs(value) {
  var number = Number(value || 0);
  if (!Number.isFinite(number) || number <= 0) return "-";
  return Math.round(number) + "ms";
}

function renderIncognitoDiagnostics(data) {
  if (incognitoDiagOutput) {
    incognitoDiagOutput.textContent = JSON.stringify(data || {}, null, 2);
  }
  var conclusion = data?.conclusion || {};
  var status = String(conclusion.status || (data?.cdp_ready ? "ready" : "cdp_not_ready"));
  var ok = !!data?.cdp_ready && status !== "cdp_not_ready" && status !== "probe_failed";
  if (incognitoDiagBadge) {
    setText(incognitoDiagBadge, status);
    incognitoDiagBadge.className = ok ? "badge" : (data?.cdp_ready ? "badge neutral" : "badge error");
  }
  setText(incognitoDiagCDP, data?.cdp_ready ? "LISTENING" : "未就绪");
  setText(incognitoDiagTargets, typeof data?.target_count === "number" ? String(data.target_count) : "-");
  setText(incognitoDiagReadyState, data?.page_probe?.ready_state || "-");
  setText(incognitoDiagLoad, formatDiagMs(data?.page_probe?.nav?.duration_ms));
  setText(incognitoDiagMessage, conclusion.message || data?.probe_error || data?.target_error || "诊断完成");

  var slow = Array.isArray(data?.page_probe?.slow_resources) ? data.page_probe.slow_resources : [];
  if (incognitoDiagSlow) {
    incognitoDiagSlow.hidden = slow.length === 0;
    incognitoDiagSlow.innerHTML = "";
    slow.slice(0, 6).forEach(function (item) {
      var row = document.createElement("div");
      row.className = "incognito-diag-slow-row";
      row.innerHTML =
        '<strong>' + escapeHTML(item.host || "unknown") + '</strong>' +
        '<span>' + escapeHTML(item.type || "resource") + " · " + formatDiagMs(item.duration_ms) + '</span>' +
        '<small>' + escapeHTML(item.path || "-") + '</small>';
      incognitoDiagSlow.appendChild(row);
    });
  }
}

async function refreshIncognitoDiagnostics(options) {
  options = options || {};
  var activeButton = options.openFirst ? incognitoDiagOpenBtn : incognitoDiagRefreshBtn;
  var originalText = activeButton?.textContent || "刷新诊断";
  if (activeButton) {
    activeButton.disabled = true;
    setText(activeButton, options.openFirst ? "打开中..." : "诊断中...");
  }
  if (incognitoDiagBadge) {
    setText(incognitoDiagBadge, "诊断中");
    incognitoDiagBadge.className = "badge neutral";
  }
  try {
    if (options.openFirst) {
      var opened = await ensureIncognitoWindow();
      if (!opened) throw new Error("无法打开无痕窗口");
      await new Promise(function (r) { setTimeout(r, 1500); });
    }
    var response = await fetch("/api/incognito/diagnostics", { method: "GET", headers: buildRequestHeaders() });
    var data = await response.json();
    renderIncognitoDiagnostics(data);
    if (!response.ok) {
      throw new Error(data.error || "无痕诊断请求失败");
    }
    return data;
  } catch (error) {
    renderIncognitoDiagnostics({ ok: false, cdp_ready: false, error: error.message, conclusion: { status: "diagnostics_failed", message: error.message || "无痕诊断失败" } });
    return null;
  } finally {
    if (activeButton) {
      activeButton.disabled = false;
      setText(activeButton, originalText);
    }
  }
}

async function runLoginClickAfterIncognitoOpen(options) {
  var returnData = !!(options && options.returnData);
  if (loginClickReady) return returnData ? { ok: true, stage: "login_click_cached_ready" } : true;
  if (luckmailResult) luckmailResult.hidden = false;
  if (luckmailBadge) { setText(luckmailBadge, "点登录中"); luckmailBadge.className = "badge neutral"; }
  setText(luckmailMailMeta, "无痕窗口已打开，正在监控首页加载、网络和登录按钮状态；若判断为网络延迟会持续等待，其他异常会自动切换处理策略...");
  announceLuckMailScene("login_click_started", "无痕窗口已打开，正在监控登录入口与页面状态");
  renderLuckMailOutput({ stage: "login_click_started" });
  try {
    const response = await fetch("/api/login/click", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ timeout_s: 45 }),
    });
    const data = await response.json();
    renderLuckMailOutput(data);
    var alreadyLoggedIn = data.stage === "already_logged_in";
    loginClickReady = !!(response.ok && data.ok && (data.email_input_ready || data.email_mode_switched || data.login_surface_ready || alreadyLoggedIn));
    var targetSwitching = data.stage && String(data.stage).includes("target_switching");
    var popupNotOpened = data.stage === "login_popup_not_opened" || !!data.login_popup_not_opened;
    var emailWatchFallback = targetSwitching || popupNotOpened || !!data.clicked_login;
    if (luckmailBadge) {
      setText(luckmailBadge, alreadyLoggedIn ? "已登录" : (loginClickReady ? "登录窗已开" : (emailWatchFallback ? "继续监控" : "点击失败")));
      luckmailBadge.className = loginClickReady || emailWatchFallback ? "badge neutral" : "badge error";
    }
    setText(luckmailMailMeta, alreadyLoggedIn ? "检测到当前无痕页已处于登录状态，无需再点登录按钮，可直接进入 Session 读取或后续流程。" : (loginClickReady ? (data.email_mode_switched ? "已点击使用电子邮箱继续，登录/注册邮箱窗口已出现。" : (data.login_surface_ready && !data.email_input_ready ? "登录/注册窗口已出现，正在进入邮箱填入步骤。" : "已点击登录按钮，登录/注册邮箱窗口已出现。")) : (targetSwitching ? "登录页面正在跳转或切换目标页，后续邮箱填入会继续监控。" : (popupNotOpened ? "登录弹窗未立即出现，已停留在当前页面；后续邮箱填入会持续监控，弹窗出现后自动填写邮箱。" : (data.error || "未检测到登录/注册邮箱窗口。")))));
    if (alreadyLoggedIn) {
      announceLuckMailScene("already_logged_in", "检测到当前页面已登录，无需再点登录按钮");
    } else if (loginClickReady) {
      announceLuckMailScene("login_window_ready", data.email_mode_switched ? "已切换为电子邮箱登录，邮箱输入窗口已出现" : "登录窗口已出现，可以填写邮箱");
    } else if (targetSwitching) {
      announceLuckMailScene("login_target_switching", "登录页面正在跳转，后续邮箱填入会继续监控");
    } else if (popupNotOpened) {
      announceLuckMailScene("login_popup_not_opened", "登录弹窗未立即出现，系统会继续监控并在弹窗出现后填写邮箱");
    } else {
      announceLuckMailScene("login_click_failed", data.error || "未检测到登录窗口，需要检查无痕页面");
    }
    return returnData ? data : loginClickReady;
  } catch (error) {
    loginClickReady = false;
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
    setText(luckmailMailMeta, error.message || "登录按钮自动点击请求失败");
    announceLuckMailScene("login_click_error", error.message || "登录按钮自动点击请求失败");
    renderLuckMailOutput({ ok: false, stage: "login_click_error", error: error.message });
    return returnData ? { ok: false, stage: "login_click_error", error: error.message } : false;
  }
}

const autoFillCheckoutBtn = document.querySelector("#autoFillCheckoutBtn");

/**
 * 确保无痕窗口已打开，若未打开则先创建 ChatGPT 首页窗口
 * 所有后续操作必须通过此函数统一管理，保证共用同一个无痕窗口
 * @returns {Promise<boolean>} 是否成功
 */
async function ensureIncognitoWindow() {
  if (incognitoWindowOpened) return true;
  const ok = await openIncognito("https://chatgpt.com/", true);
  if (!ok) return false;
  await new Promise(function (r) { setTimeout(r, 1500); });
  return true;
}

async function openChatGPTInSameIncognito() {
  return await openIncognito("https://chatgpt.com/", false);
}

async function waitForSessionLoginCompletion(maxWaitMs) {
  var startedAt = Date.now();
  while (Date.now() - startedAt < maxWaitMs) {
    await new Promise(function (r) { setTimeout(r, 2500); });
    try {
      var result = await fetchLatestSessionJSON({ skipEnsureWindow: true, allowRecovery: false, silentConclusion: true });
      if (result) {
        return result;
      }
    } catch (_err) {
    }
  }
  throw new Error("等待登录超时，请在无痕窗口中完成 ChatGPT 登录后重试。");
}

async function handleSessionAutoAction(conclusion) {
  if (!conclusion) return null;
  var action = conclusion.auto_action || "";

  if (action === "continue_checkout" || action === "manual_review") {
    return null;
  }

  if (action === "open_chatgpt_home") {
    appendMonitorEvent({ domain: "Log", method: "session-auto-action", summary: "自动恢复：打开 chatgpt.com", ts: Date.now() });
    var opened = await openChatGPTInSameIncognito();
    if (!opened) {
      throw new Error("自动打开 chatgpt.com 失败，请手动检查无痕窗口。");
    }
    await new Promise(function (r) { setTimeout(r, 1800); });
    return await fetchLatestSessionJSON({ skipEnsureWindow: true, allowRecovery: false, silentConclusion: true });
  }

  if (action === "wait_and_retry" || action === "retry_session_fetch") {
    appendMonitorEvent({ domain: "Log", method: "session-auto-action", summary: "自动恢复：等待后重试 Session 获取", ts: Date.now() });
    await new Promise(function (r) { setTimeout(r, 2000); });
    return await fetchLatestSessionJSON({ skipEnsureWindow: true, allowRecovery: false, silentConclusion: true });
  }

  if (action === "wait_for_login") {
    appendMonitorEvent({ domain: "Log", method: "session-auto-action", summary: "自动恢复：等待你在无痕窗口完成登录", ts: Date.now() });
    if (monitorBadge) { monitorBadge.textContent = "等待登录中"; monitorBadge.className = "badge"; }
    return await waitForSessionLoginCompletion(120000);
  }

  return null;
}

/**
 * 自动从当前无痕 Chrome 会话提取最新 Session JSON，并回填到左侧输入框
 * @returns {Promise<string>} 最新 Session JSON 文本
 */
async function fetchLatestSessionJSON(options) {
  options = options || {};
  const skipEnsureWindow = !!options.skipEnsureWindow;
  const allowRecovery = options.allowRecovery !== false;
  const silentConclusion = !!options.silentConclusion;

  if (!skipEnsureWindow) {
    const ensured = await ensureIncognitoWindow();
    if (!ensured) {
      throw new Error("无法启动 Chrome 无痕窗口，请确认系统已安装 Chrome 浏览器。");
    }
  }

  sessionMonitorAbortController = new AbortController();
  clearMonitor();
  if (gopayMonitor) gopayMonitor.hidden = false;
  if (monitorBadge) { monitorBadge.textContent = "Session 监控中"; monitorBadge.className = "badge"; }

  const response = await fetch("/api/session/fetch", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify({ stream: true }),
    signal: sessionMonitorAbortController.signal,
  });

  if (!response.ok) {
    var errorText = "";
    try {
      errorText = await response.text();
    } catch (_err) {
    }
    throw new Error(errorText || "获取 Session JSON 失败，请确认已在无痕窗口中登录 ChatGPT");
  }

  if (!response.body) {
    throw new Error("浏览器未返回可读取的流式响应，请刷新页面后重试。");
  }

  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let fullOutput = [];
  let resultJSON = "";
  let diagnostics = null;
  let finalError = null;

  while (true) {
    const chunk = await reader.read();
    if (chunk.done) break;
    buffer += decoder.decode(chunk.value, { stream: true });
    const lines = buffer.split("\n");
    buffer = lines.pop() || "";

    for (let i = 0; i < lines.length; i++) {
      const line = lines[i].trim();
      if (!line) continue;
      const msg = JSON.parse(line);
      fullOutput.push(msg);

      if (monitorOutput) {
        monitorOutput.hidden = false;
        monitorOutput.textContent = JSON.stringify(fullOutput, null, 2);
      }

      if (msg.type === "started") {
        appendMonitorEvent({ domain: "Log", method: "session-monitor-started", summary: (msg.targets || []).join(" | "), ts: Date.now() });
      } else if (msg.type === "event" && msg.event) {
        appendMonitorEvent(msg.event);
      } else if (msg.type === "data" || msg.type === "done") {
        resultJSON = msg.data?.json || resultJSON;
        diagnostics = msg.data?.diagnostics || diagnostics;
      } else if (msg.type === "error") {
        diagnostics = msg.data || diagnostics;
        finalError = msg.error || "获取 Session JSON 失败";
        appendMonitorEvent({ domain: "Error", method: "session-fetch-error", summary: finalError, ts: Date.now() });
      }
    }
  }

  sessionMonitorAbortController = null;

  if (diagnostics && monitorOutput) {
    monitorOutput.hidden = false;
    monitorOutput.textContent = JSON.stringify({ stream: fullOutput, diagnostics: diagnostics }, null, 2);
  }

  if (diagnostics?.conclusion && !silentConclusion) {
    renderSessionConclusion(diagnostics.conclusion);
  }

  const sessionAccessToken = extractSessionAccessToken(resultJSON);
  const sessionReady = diagnostics?.has_access_token || diagnostics?.conclusion?.status === "session_ready";
  if (resultJSON && (sessionReady || sessionAccessToken)) {
    fields.token.value = resultJSON;
    if (sessionText) setText(sessionText, "已提取 Session JSON");
    if (monitorBadge) { monitorBadge.textContent = "Session 已获取"; monitorBadge.className = "badge"; }
    return resultJSON;
  }

  if (resultJSON && !sessionAccessToken) {
    appendMonitorEvent({
      domain: "Error",
      method: "session-missing-access-token",
      summary: diagnostics?.conclusion?.message || "Session 响应没有 accessToken，本次不作为成功结果",
      ts: Date.now(),
    });
  }

  if (diagnostics?.conclusion && allowRecovery) {
    var recovered = await handleSessionAutoAction(diagnostics.conclusion);
    if (recovered) {
      return recovered;
    }
  }

  if (monitorBadge) { monitorBadge.textContent = "Session 失败"; monitorBadge.className = "badge error"; }
  throw new Error(finalError || diagnostics?.conclusion?.message || diagnostics?.session_preview || "获取 Session JSON 失败，请确认已在无痕窗口中登录 ChatGPT");
}

function renderSessionConclusion(conclusion) {
  if (!conclusion) return;
  if (conclusion.message) {
    appendMonitorEvent({ domain: "Log", method: "session-conclusion", summary: conclusion.message, ts: Date.now() });
  }
  if (alertBox) {
    alertBox.hidden = false;
    alertBox.classList.remove("cookie", "token");
    if (alertTitle) setText(alertTitle, "Session 诊断结论");
    if (alertBadge) setText(alertBadge, conclusion.status || "诊断");
    if (alertMessage) setText(alertMessage, conclusion.message || "-");
    if (alertHint) {
      alertHint.hidden = false;
      setText(alertHint, conclusion.status || "-");
    }
    renderAlertSteps(Array.isArray(conclusion.next_steps) ? conclusion.next_steps : []);
  }
}

function showAutoFillNotice(title, badge, message, hint, kind) {
  if (!alertBox) {
    return;
  }
  alertBox.hidden = false;
  alertBox.classList.remove("cookie", "token");
  if (alertTitle) setText(alertTitle, title || "自动填地址");
  if (alertBadge) setText(alertBadge, badge || "提示");
  if (alertMessage) setText(alertMessage, message || "-");
  if (alertHint) {
    alertHint.hidden = !hint;
    setText(alertHint, hint || "-");
  }
  if (alertSteps) alertSteps.hidden = true;
  if (alertStepsList) alertStepsList.innerHTML = "";
  if (kind === "error") {
    alertBox.classList.add("token");
  }
}

async function resolveOpenedCheckoutTarget(openedURL, options) {
  options = options || {};
  var deadline = Date.now() + (options.timeoutMS || 5000);
  var delayMS = options.initialDelayMS || 0;
  var lastData = null;
  var traceID = ensureAutomationTraceID("checkout");
  var openedCheckoutKey = typeof checkoutAutoTriggerKey === "function" ? checkoutAutoTriggerKey(openedURL) : "";

  while (Date.now() <= deadline) {
    if (delayMS > 0) {
      await new Promise(function (r) { setTimeout(r, delayMS); });
    }
    try {
      const resp = await fetch("/api/checkout/resolve-target", {
        method: "POST",
        headers: buildRequestHeaders(),
        body: JSON.stringify({
          opened_url: openedURL,
          target_id: latestOpenedCheckoutTargetID,
          trace_id: traceID,
        }),
      });
      const data = await resp.json();
      lastData = data;
      if (resp.ok && data.ok) {
        var currentURL = data.current_url || data.target?.url || "";
        var currentCheckoutKey = typeof checkoutAutoTriggerKey === "function" ? checkoutAutoTriggerKey(currentURL) : "";
        var acceptsResolvedTarget = !data.fallback || !openedCheckoutKey || currentCheckoutKey === openedCheckoutKey;
        if (acceptsResolvedTarget) {
          if (data.current_url) latestOpenedCheckoutURL = data.current_url;
          if (data.target?.id || data.target_id) latestOpenedCheckoutTargetID = data.target?.id || data.target_id;
          return data;
        }
      }
    } catch (error) {
      console.debug("resolve checkout target retry:", error);
    }
    delayMS = Math.min(delayMS > 0 ? delayMS * 2 : 250, 500);
  }
  return lastData;
}

/**
 * 在已有的无痕窗口中打开指定 URL（不创建新窗口）
 * @param {string} url - 要打开的 URL
 */
async function openInSameIncognito(url) {
  const ensured = await ensureIncognitoWindow();
  if (!ensured) {
    showAutoFillNotice("打开无痕窗口失败", "错误", "无法启动 Chrome 无痕窗口，请确认系统已安装 Chrome 浏览器。", "", "error");
    return false;
  }
  const opened = await openIncognito(url, false);
  if (!opened) {
    return false;
  }
  latestOpenedCheckoutURL = url;
  latestOpenedCheckoutTargetID = "";
  try {
    const data = await resolveOpenedCheckoutTarget(url, { timeoutMS: 5000, initialDelayMS: 250 });
    if (data?.ok && !data.fallback && (data.current_url || data.target?.id || data.target_id)) {
      if (data.current_url) latestOpenedCheckoutURL = data.current_url;
      if (data.target?.id || data.target_id) latestOpenedCheckoutTargetID = data.target?.id || data.target_id;
    } else {
      console.error("resolve checkout target failed:", data);
    }
  } catch (error) {
    console.error("resolve checkout target error:", error);
  }
  return true;
}

async function autoSelectOpenedCheckoutPaymentMethod(expectedURL) {
  if (!expectedURL) return null;
  try {
    const resp = await fetch("/api/checkout/payment-method-select", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({
        expected_url: expectedURL,
        target_id: latestOpenedCheckoutTargetID,
        trace_id: ensureAutomationTraceID("checkout"),
      }),
    });
    const data = await resp.json();
    if (data?.target?.id || data?.target_id) {
      latestOpenedCheckoutTargetID = data.target?.id || data.target_id;
    }
    if (data?.current_url) {
      latestOpenedCheckoutURL = data.current_url;
    }
    if (resp.ok && data.ok) {
      var selected = data.selected_payment_method === "paypal" ? "PayPal" : "GoPay";
      showAutoFillNotice("支付方式已选择", "完成", "已自动选择 " + selected + " 支付方式。", "如页面没有 GoPay，系统会自动选择 PayPal；最终订阅仍需你本人确认。", "success");
    } else {
      showAutoFillNotice("支付方式未自动选择", "提示", data.error || "未检测到可选 GoPay 或 PayPal。", "请在支付页面手动选择支付方式。", "warning");
    }
    return data;
  } catch (error) {
    console.error("auto payment method select error:", error);
    return null;
  }
}

/**
 * 在无痕窗口中打开支付链接
 */
async function openPaymentInIncognito() {
  if (!latestCheckoutURL) return false;
  var opened = await openInSameIncognito(latestCheckoutURL);
  if (opened) {
    await autoSelectOpenedCheckoutPaymentMethod(latestOpenedCheckoutURL || latestCheckoutURL);
  }
  return opened;
}

const PLUS_SUBSCRIBE_WATCH_INITIAL_DELAY_MS = 1800;
const PLUS_SUBSCRIBE_WATCH_MIN_INTERVAL_MS = 3500;
const PLUS_SUBSCRIBE_WATCH_MAX_INTERVAL_MS = 60000;
const PLUS_SUBSCRIBE_WATCH_WINDOW_MS = 8 * 60 * 1000;
const PLUS_SUBSCRIBE_TRIGGER_COOLDOWN_MS = 30000;

function plusSubscribeProbeSignature(data) {
  return [
    data?.url || data?.target?.url || "",
    data?.button_text || "",
    data?.title || "",
  ].join("|").slice(0, 500);
}

function stopPlusSubscribeSessionTriggerWatcher() {
  if (plusSubscribeWatcherTimer) {
    window.clearTimeout(plusSubscribeWatcherTimer);
    plusSubscribeWatcherTimer = null;
  }
}

function schedulePlusSubscribeSessionTriggerWatcher(reason, delayMS) {
  if (!fetchSessionBtn || Date.now() > plusSubscribeWatcherExpiresAt) {
    stopPlusSubscribeSessionTriggerWatcher();
    return;
  }
  if (plusSubscribeWatcherTimer) {
    window.clearTimeout(plusSubscribeWatcherTimer);
  }
  plusSubscribeWatcherReason = reason || plusSubscribeWatcherReason || "watch";
  plusSubscribeWatcherTimer = window.setTimeout(function () {
    plusSubscribeWatcherTimer = null;
    void probePlusSubscribeSessionTrigger(plusSubscribeWatcherReason);
  }, Math.max(0, delayMS || PLUS_SUBSCRIBE_WATCH_MIN_INTERVAL_MS));
}

function startPlusSubscribeSessionTriggerWatcher(reason) {
  if (!fetchSessionBtn) return;
  plusSubscribeWatcherReason = reason || plusSubscribeWatcherReason || "watch";
  plusSubscribeWatcherExpiresAt = Math.max(plusSubscribeWatcherExpiresAt, Date.now() + PLUS_SUBSCRIBE_WATCH_WINDOW_MS);
  if (!plusSubscribeWatcherTimer && !plusSubscribeProbeInFlight) {
    plusSubscribeProbeConsecutiveMisses = 0;
    plusSubscribeProbeLastSignature = "";
    schedulePlusSubscribeSessionTriggerWatcher(plusSubscribeWatcherReason, PLUS_SUBSCRIBE_WATCH_INITIAL_DELAY_MS);
  }
}

function nextPlusSubscribeProbeDelay(data) {
  const signature = plusSubscribeProbeSignature(data || {});
  if (signature && signature !== plusSubscribeProbeLastSignature) {
    plusSubscribeProbeConsecutiveMisses = 0;
    plusSubscribeProbeLastSignature = signature;
  } else {
    plusSubscribeProbeConsecutiveMisses += 1;
  }
  if (data?.safe_trigger_fetch_session) {
    return PLUS_SUBSCRIBE_WATCH_MIN_INTERVAL_MS;
  }
  const stage = String(data?.stage || "");
  const baseDelay = (!data?.cdp_ready || data?.chatgpt_target_count === 0 || stage === "chatgpt_target_not_found") ? 10000 : PLUS_SUBSCRIBE_WATCH_MIN_INTERVAL_MS;
  const multiplier = Math.pow(1.55, Math.min(plusSubscribeProbeConsecutiveMisses, 7));
  return Math.min(PLUS_SUBSCRIBE_WATCH_MAX_INTERVAL_MS, Math.round(baseDelay * multiplier));
}

async function probePlusSubscribeSessionTrigger(reason, options) {
  options = options || {};
  if (plusSubscribeProbeInFlight) return null;
  if (Date.now() > plusSubscribeWatcherExpiresAt) {
    stopPlusSubscribeSessionTriggerWatcher();
    return null;
  }
  if (autoFetchAndGenerateRunning || sessionMonitorAbortController) {
    if (!options.singleShot) {
      schedulePlusSubscribeSessionTriggerWatcher(reason || "watch", PLUS_SUBSCRIBE_WATCH_MIN_INTERVAL_MS);
    }
    return null;
  }
  plusSubscribeProbeInFlight = true;
  let data = null;
  let keepWatching = !options.singleShot;
  try {
    const response = await fetch("/api/pricing/plus-subscribe-probe", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ reason: reason || "watch", trace_id: ensureAutomationTraceID("plus") }),
    });
    data = await response.json();
    if (!response.ok || !data?.safe_trigger_fetch_session) {
      return data;
    }
    const signature = plusSubscribeProbeSignature(data);
    if (signature && signature === plusSubscribeAutoTriggeredSignature) {
      return data;
    }
    if (Date.now() - plusSubscribeAutoTriggerLastAttemptAt < PLUS_SUBSCRIBE_TRIGGER_COOLDOWN_MS) {
      return data;
    }
    plusSubscribeAutoTriggerLastAttemptAt = Date.now();
    appendMonitorEvent({
      domain: "Log",
      method: "plus-subscribe-detected",
      summary: "检测到 Plus 套餐与订阅并付款按钮，自动触发获取 Session JSON 流程",
      ts: Date.now(),
    });
    showAutoFillNotice(
      "检测到 Plus 订阅确认",
      "自动",
      "已识别「Plus 套餐 / 订阅并付款」页面，正在自动获取 Session JSON 并打开订阅支付页。",
      "系统不会自动点击最终订阅付款按钮。",
      "success"
    );
    const completed = await autoFetchAndGenerate({ source: "plus_subscribe_card" });
    if (completed && signature) {
      plusSubscribeAutoTriggeredSignature = signature;
      stopPlusSubscribeSessionTriggerWatcher();
      keepWatching = false;
    }
    return data;
  } catch (error) {
    console.debug("plus subscribe probe skipped:", error);
    return null;
  } finally {
    plusSubscribeProbeInFlight = false;
    if (keepWatching && Date.now() <= plusSubscribeWatcherExpiresAt && !plusSubscribeWatcherTimer) {
      schedulePlusSubscribeSessionTriggerWatcher(reason || "watch", nextPlusSubscribeProbeDelay(data));
    }
  }
}

async function bootstrapPlusSubscribeSessionTriggerWatcher() {
  plusSubscribeWatcherExpiresAt = Date.now() + 10000;
  var data = await probePlusSubscribeSessionTrigger("startup_probe", { singleShot: true });
  if (data?.cdp_ready && data?.chatgpt_target_count > 0) {
    startPlusSubscribeSessionTriggerWatcher("startup_existing_window");
  } else {
    plusSubscribeWatcherExpiresAt = 0;
    stopPlusSubscribeSessionTriggerWatcher();
  }
}

function resetResult() {
  latestCheckoutURL = "";
  latestOpenedCheckoutURL = "";
  latestOpenedCheckoutTargetID = "";
  activeAutomationTraceID = createAutomationTraceID("checkout");
  setText(statusText, "生成中");
  setText(sessionText, "-");
  setText(hostText, "-");
  setText(finalTransactionText, "请求上游");
  setText(finalSettlementText, "-");
  setText(output, "{}");
  setStepState("input", "active", "正在识别输入内容");
  setStepState("token", "", "等待提取 token");
  setStepState("checkout", "", "等待请求 checkout");
  setStepState("cookie", "", "等待判断是否需要浏览器会话");
  setStepState("link", "", "等待生成最终链接");
  if (alertBox) {
    alertBox.hidden = true;
    alertBox.classList.remove("cookie", "token");
  }
  if (alertHint) {
    alertHint.hidden = true;
    setText(alertHint, "-");
  }
  if (alertSteps) {
    alertSteps.hidden = true;
  }
  if (alertStepsList) {
    alertStepsList.innerHTML = "";
  }
  if (checkoutUrlLink) {
    checkoutUrlLink.hidden = true;
    checkoutUrlLink.href = "#";
    setText(checkoutUrlLink, "打开链接");
  }
  if (copyCheckoutLinkButton) {
    copyCheckoutLinkButton.hidden = true;
    copyCheckoutLinkButton.disabled = true;
    setText(copyCheckoutLinkButton, "复制链接");
  }
  if (openInIncognitoBtn) {
    openInIncognitoBtn.hidden = true;
    openInIncognitoBtn.disabled = true;
  }
  if (openPaymentShortcutBtn) {
    openPaymentShortcutBtn.disabled = true;
  }
}

function renderAlertSteps(steps) {
  if (!alertSteps || !alertStepsList) {
    return;
  }
  alertStepsList.innerHTML = "";
  if (!Array.isArray(steps) || steps.length === 0) {
    alertSteps.hidden = true;
    return;
  }
  for (const step of steps) {
    const item = document.createElement("li");
    item.textContent = step;
    alertStepsList.append(item);
  }
  alertSteps.hidden = false;
}

function setStepState(step, state, text) {
  const item = stepFlowItems.find((node) => node.dataset.step === step);
  if (!item) {
    return;
  }
  item.classList.remove("active", "done", "error", "warning");
  if (state) {
    item.classList.add(state);
  }
  const textMap = {
    input: stepInputText,
    token: stepTokenText,
    checkout: stepCheckoutText,
    cookie: stepCookieText,
    link: stepLinkText,
  };
  const target = textMap[step];
  setText(target, text || "-");
}

function renderStepFlow(data, checkoutURL) {
  const inputValue = fields.token?.value?.trim() || "";
  const looksLikeSessionJSON = inputValue.startsWith("{") && inputValue.includes("accessToken");

  setStepState("input", "done", looksLikeSessionJSON ? "已识别为 Session JSON" : (inputValue ? "已识别为手动输入" : "等待提交"));
  setStepState("token", data?.token_invalidated ? "error" : "done", data?.token_invalidated ? "提取成功，但 token 已失效" : "已提取可用 token 格式");

  if (data?.stage === "checkout_upstream" && data?.status === 401) {
    setStepState("checkout", "error", "上游拒绝了当前 token");
  } else if (data?.stage === "checkout_upstream" && data?.status === 403) {
    setStepState("checkout", "warning", "checkout 已返回 403，需要浏览器态");
  } else if (data?.stage === "checkout_trial_validation") {
    setStepState("checkout", "done", "checkout 会话已拿到，正在校验试用资格");
  } else if (data?.stage === "checkout_link") {
    setStepState("checkout", "done", "checkout 与 Stripe 初始化已完成");
  } else if (checkoutURL) {
    setStepState("checkout", "done", "checkout 初始化成功");
  } else {
    setStepState("checkout", "active", "等待上游返回 checkout 结果");
  }

  if (data?.requires_cookie) {
    setStepState("cookie", "warning", "当前账号需要补 chatgpt.com Cookie");
  } else if (data?.stage === "checkout_upstream" && data?.status === 401) {
    setStepState("cookie", "", "此轮失败不是 Cookie 问题");
  } else if (checkoutURL || data?.stage === "checkout_link" || data?.stage === "checkout_trial_validation") {
    setStepState("cookie", "done", "当前请求未被 Cookie 阻断");
  } else {
    setStepState("cookie", "", "如遇 403，再补 Cookie");
  }

  if (checkoutURL) {
    setStepState("link", "done", "已生成有效支付链接");
  } else if (data?.stage === "checkout_trial_validation") {
    setStepState("link", "warning", "卡在 0 元试用资格校验");
  } else if (data?.stage === "checkout_link") {
    setStepState("link", "error", "Stripe 已返回，但未提取到可用链接");
  } else if (data?.stage === "checkout_upstream") {
    setStepState("link", "", "上游未进入可生成链接阶段");
  } else {
    setStepState("link", "active", "等待生成最终支付链接");
  }
}

function errorPresentation(data) {
  if (data?.token_invalidated) {
    return {
      type: "token",
      title: "登录凭证已失效",
      badge: "Token",
      message: "当前 access token 已失效，系统无法继续 Popay。",
      hint: data.hint || "请重新登录 ChatGPT，并重新粘贴新的 access token 或完整 /api/auth/session JSON。",
      steps: [
        "在浏览器里重新登录 ChatGPT，确保当前账号处于已登录状态。",
        "打开 /api/auth/session 或获取新的 access token。",
        "把新的 access token 或整份 session JSON 粘贴回左侧输入框后重试。",
      ],
    };
  }
  if (data?.requires_cookie) {
    return {
      type: "cookie",
      title: "需要浏览器会话",
      badge: "Cookie",
      message: "当前账号在初始化 checkout 时需要 chatgpt.com 的浏览器会话 Cookie。",
      hint: data.hint || "请在“可选浏览器态”里补充当前 chatgpt.com 会话 Cookie，必要时再补 User-Agent。",
      steps: [
        "在当前已登录 ChatGPT 的浏览器里打开 chatgpt.com。",
        "按 F12 打开开发者工具，进入 Network 或 Application / Storage 查看 Cookie。",
        "复制 chatgpt.com 当前会话相关 Cookie，粘贴到左侧“可选浏览器态”里的 Cookie 输入框。",
        "如果仍失败，再把当前浏览器的 User-Agent 一并填入后重试。",
      ],
    };
  }
  return {
    type: "generic",
    title: activePaymentLabel + " 失败",
    badge: "失败",
    message: data?.error || "请求未成功完成。",
    hint: data?.hint || "请检查输入参数、网络环境或上游返回内容。",
    steps: [
      "先确认 access token 是否最新且未失效。",
      "如果返回 403，再补 chatgpt.com 的 Cookie。",
      "如仍失败，请查看下方原始返回内容继续排查。",
    ],
  };
}

function renderResult(data, elapsedMs, ok) {
  const checkoutURL = data.checkout_url || data.url || "";
  latestCheckoutURL = checkoutURL;
  if (data?.trace_id) {
    activeAutomationTraceID = data.trace_id;
  }

  let host = "-";
  if (checkoutURL) {
    try {
      host = new URL(checkoutURL).hostname || "-";
    } catch (error) {
      host = "-";
    }
  }

  setText(latency, `${elapsedMs}ms`);
  latency?.classList.toggle("error", !ok);
  latency?.classList.toggle("neutral", ok);
  setText(statusText, checkoutURL ? "checkout_link_ready" : (data.stage || data.error || "failed"));
  setText(sessionText, data.checkout_session_id || data.session_id || "-");
  setText(hostText, host);
  setText(finalTransactionText, checkoutURL ? "生成成功" : "生成失败");
  setText(finalSettlementText, checkoutURL ? "只返回官方长链接" : (data.hint || data.error || "-"));
  setText(output, checkoutURL || JSON.stringify(data, null, 2));
  renderStepFlow(data, checkoutURL);

  if (alertBox) {
    if (checkoutURL) {
      alertBox.hidden = true;
      alertBox.classList.remove("cookie", "token");
    } else {
      const presentation = errorPresentation(data);
      alertBox.hidden = false;
      alertBox.classList.remove("cookie", "token");
      if (presentation.type === "cookie") {
        alertBox.classList.add("cookie");
      }
      if (presentation.type === "token") {
        alertBox.classList.add("token");
      }
      setText(alertTitle, presentation.title);
      setText(alertBadge, presentation.badge);
      setText(alertMessage, presentation.message);
      setText(alertHint, presentation.hint || "-");
      renderAlertSteps(presentation.steps);
      if (alertHint) {
        alertHint.hidden = !presentation.hint;
      }
    }
  }

  if (checkoutUrlLink) {
    checkoutUrlLink.hidden = !checkoutURL;
    checkoutUrlLink.href = checkoutURL || "#";
    setText(checkoutUrlLink, checkoutURL || "打开链接");
  }
  if (copyCheckoutLinkButton) {
    copyCheckoutLinkButton.hidden = !checkoutURL;
    copyCheckoutLinkButton.disabled = !checkoutURL;
    setText(copyCheckoutLinkButton, "复制链接");
  }
  if (openInIncognitoBtn) {
    openInIncognitoBtn.hidden = !checkoutURL;
    openInIncognitoBtn.disabled = !checkoutURL;
  }
  if (openPaymentShortcutBtn) {
    openPaymentShortcutBtn.disabled = !checkoutURL;
  }
}

async function runCheckoutGenerate(activeSubmitButton, options) {
  options = options || {};
  if (!options.paymentMethod && activeSubmitButton?.dataset?.paymentMethod) {
    options.paymentMethod = activeSubmitButton.dataset.paymentMethod;
  }
  resetResult();
  paymentSubmitButtons.forEach(function (button) { button.disabled = true; });
  var activeSubmitLabel = activeSubmitButton ? activeSubmitButton.textContent : "Popay";
  activePaymentLabel = activeSubmitLabel || "Popay";
  setText(activeSubmitButton, options.fetchSessionFirst ? "读取 Session..." : "生成中");
  setText(latency, options.fetchSessionFirst ? "读取 Session" : "运行中");

  const startedAt = performance.now();
  try {
    if (options.fetchSessionFirst) {
      await fetchLatestSessionJSON();
      setText(activeSubmitButton, "生成中");
      setText(latency, "运行中");
    }
    const { response, data } = await postJSON("/api/checkout/start", buildStartPayload(options));
    renderResult(data, Math.round(performance.now() - startedAt), response.ok);
    if (response.ok && latestCheckoutURL) {
      void copyCurrentCheckoutURL();
      void openPaymentInIncognito();
    }
  } catch (error) {
    renderResult({ error: error.message }, Math.round(performance.now() - startedAt), false);
  } finally {
    paymentSubmitButtons.forEach(function (button) { button.disabled = false; });
    setText(activeSubmitButton, activeSubmitLabel);
  }
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  var activeSubmitButton = event.submitter && paymentSubmitButtons.includes(event.submitter) ? event.submitter : submitButton;
  await runCheckoutGenerate(activeSubmitButton, { fetchSessionFirst: false });
});

paypalSubmitButton?.addEventListener("click", async () => {
  await runCheckoutGenerate(paypalSubmitButton, {
    paymentMethod: "paypal",
    fetchSessionFirst: true,
    billingDetails: {
      country: "US",
      currency: "USD",
    },
  });
});

/**
 * 全自动管道：提取 Session JSON → 粘贴到输入框 → 自动生成支付链接 → 自动打开
 */
async function autoFetchAndGenerate(options) {
  options = options || {};
  if (!fetchSessionBtn || autoFetchAndGenerateRunning) return false;
  autoFetchAndGenerateRunning = true;
  fetchSessionBtn.disabled = true;
  setWorkflowButtonState(fetchSessionBtn, "running");
  const origText = "获取 Session JSON";
  setText(fetchSessionBtn, "启动窗口...");

  try {
    if (options.source === "plus_subscribe_card") {
      appendMonitorEvent({ domain: "Log", method: "session-auto-trigger", summary: "Plus 订阅确认页触发获取 Session JSON", ts: Date.now() });
    }
    setText(fetchSessionBtn, "监控并读取 Session...");
    await fetchLatestSessionJSON();
    setWorkflowButtonState(fetchSessionBtn, "done");
    setText(fetchSessionBtn, "生成链接...");
    form.dispatchEvent(new Event("submit", { cancelable: true, bubbles: true }));
    return true;
  } catch (error) {
    if (monitorBadge) { monitorBadge.textContent = "Session 失败"; monitorBadge.className = "badge error"; }
    setWorkflowButtonState(fetchSessionBtn, "error");
    showAutoFillNotice("获取 Session 失败", "错误", error.message || "获取 Session JSON 失败，请确认已在无痕窗口中登录 ChatGPT", "", "error");
    return false;
  } finally {
    sessionMonitorAbortController = null;
    fetchSessionBtn.disabled = false;
    setText(fetchSessionBtn, origText);
    autoFetchAndGenerateRunning = false;
  }
}

copyCheckoutLinkButton?.addEventListener("click", () => {
  void copyCurrentCheckoutURL();
});

openInIncognitoBtn?.addEventListener("click", () => {
  void openPaymentInIncognito();
});

openPaymentShortcutBtn?.addEventListener("click", async () => {
  if (!latestCheckoutURL) {
    setWorkflowButtonState(openPaymentShortcutBtn, "error");
    showAutoFillNotice("打开支付页失败", "提示", "还没有可打开的支付链接。", "请先完成获取 Session JSON 和生成链接步骤。", "error");
    return;
  }
  setWorkflowButtonState(openPaymentShortcutBtn, "running");
  var opened = await openPaymentInIncognito();
  setWorkflowButtonState(openPaymentShortcutBtn, opened ? "done" : "error");
});

openIncognitoBtn?.addEventListener("click", async () => {
  if (!openIncognitoBtn) return;
  var originalText = openIncognitoBtn.textContent || "打开无痕窗口";
  openIncognitoBtn.disabled = true;
  setWorkflowButtonState(openIncognitoBtn, "running");
  setText(openIncognitoBtn, "打开中...");
  try {
    var opened = await ensureIncognitoWindow();
    if (!opened) {
      setWorkflowButtonState(openIncognitoBtn, "error");
      showAutoFillNotice("打开无痕窗口失败", "错误", "无法启动 Chrome 无痕窗口，请确认系统已安装 Chrome 浏览器。", "", "error");
      return;
    }
    setWorkflowButtonState(openIncognitoBtn, "done");
    showAutoFillNotice("无痕窗口已打开", "完成", "已打开 ChatGPT 无痕窗口。", "下一步请执行「设备登录引导」。", "success");
  } finally {
    openIncognitoBtn.disabled = false;
    setText(openIncognitoBtn, originalText);
  }
});

fetchSessionBtn?.addEventListener("click", () => {
  void autoFetchAndGenerate();
});

incognitoDiagRefreshBtn?.addEventListener("click", () => {
  void refreshIncognitoDiagnostics();
});

incognitoDiagOpenBtn?.addEventListener("click", () => {
  void refreshIncognitoDiagnostics({ openFirst: true });
});

exportSub2APIBtn?.addEventListener("click", () => {
  void exportSub2APICredentials();
});

closeIncognitoBtn?.addEventListener("click", () => {
  void closeIncognitoWindow();
});

autoFillCheckoutBtn?.addEventListener("click", async () => {
  if (!autoFillCheckoutBtn) return;
  autoFillCheckoutBtn.disabled = true;
  setWorkflowButtonState(autoFillCheckoutBtn, "running");
  var origText = "自动填地址";
  setText(autoFillCheckoutBtn, "探测表单...");

  try {
    var ensured = await ensureIncognitoWindow();
    if (!ensured) {
      setWorkflowButtonState(autoFillCheckoutBtn, "error");
      showAutoFillNotice("自动填地址失败", "错误", "无法启动 Chrome 无痕窗口", "", "error");
      return;
    }
    if (!latestOpenedCheckoutURL) {
      setWorkflowButtonState(autoFillCheckoutBtn, "error");
      showAutoFillNotice("自动填地址失败", "错误", "未找到本工具最近一次打开的支付链接页面。", "请先用本工具打开支付链接后再试。", "error");
      return;
    }

    setText(autoFillCheckoutBtn, "生成地址...");
    await new Promise(function (r) { setTimeout(r, 500); });

    var resp = await fetch("/api/checkout/auto-fill", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({
        expected_url: latestOpenedCheckoutURL,
        target_id: latestOpenedCheckoutTargetID,
        trace_id: ensureAutomationTraceID("checkout"),
      }),
    });
    var data = await resp.json();
    if (data?.page_target?.id || data?.target_id) {
      latestOpenedCheckoutTargetID = data.page_target?.id || data.target_id;
    }
    if (data?.current_url) {
      latestOpenedCheckoutURL = data.current_url;
    }

    if (resp.ok && data.ok) {
      var a = data.address;
      var fields = data.filled || {};
      var validation = data.address_validation || fields.validation || {};
      var selectedPayment = data.selected_payment_method === "paypal" ? "PayPal" : (data.selected_payment_method === "gopay" ? "GoPay" : "");
      var msg = "随机美国地址已填入：" + "\n" +
        "姓名: " + (a.first_name || "") + " " + (a.last_name || "") + "\n" +
        "地址: " + (a.line1 || "") + "\n" +
        "城市: " + (a.city || "") + ", " + (a.state || "") + " " + (a.zip_code || "") + "\n\n" +
        "支付方式: " + (selectedPayment ? ("已选择 " + selectedPayment) : "已保持当前选择") + "\n" +
        "地址校验: " + (validation.ok === false ? "未通过" : "通过") + "\n" +
        "填入结果: " + JSON.stringify(fields);
      showAutoFillNotice("安全辅助完成", "成功", msg, "请你本人在支付页面确认条款复选框，并手动点击订阅。系统会在提交后自动触发 GoPay 一键绑定。", "");
      setWorkflowButtonState(autoFillCheckoutBtn, "done");
      setText(autoFillCheckoutBtn, "已填入 ✓");
      startCheckoutSubmitWatcher(data);
    } else {
      setWorkflowButtonState(autoFillCheckoutBtn, "error");
      showAutoFillNotice("自动填地址失败", "错误", data.error || "自动填地址失败", "请先在无痕窗口中手动进入 ChatGPT Plus 升级结账页面。", "error");
    }
  } catch (err) {
    setWorkflowButtonState(autoFillCheckoutBtn, "error");
    showAutoFillNotice("自动填地址失败", "错误", "网络错误: " + (err.message || "未知"), "", "error");
  } finally {
    autoFillCheckoutBtn.disabled = false;
    setText(autoFillCheckoutBtn, origText);
  }
});

// =================================
// GoPay 一键绑定
// =================================
const gopayAccountId = document.querySelector("#gopayAccountId");
const gopayCountryCode = document.querySelector("#gopayCountryCode");
const gopayPhone = document.querySelector("#gopayPhone");
const gopayOTPChannel = document.querySelector("#gopayOTPChannel");
const gopayOTPInput = document.querySelector("#gopayOTP");
const gopayPINCodeInput = document.querySelector("#gopayPINCode");
const gopayAggressiveRetryInput = document.querySelector("#gopayAggressiveRetry");
const gopayLinkBtn = document.querySelector("#gopayLinkBtn");
const gopayBadge = document.querySelector("#gopayBadge");
const gopayStepFlow = document.querySelector("#gopayStepFlow");
const gopayResult = document.querySelector("#gopayResult");
const gopayLatency = document.querySelector("#gopayLatency");
const gopayLinkStatus = document.querySelector("#gopayLinkStatus");
const gopayRefID = document.querySelector("#gopayRefID");
const gopayOutput = document.querySelector("#gopayOutput");
let gopayPaymentFlowRunning = false;
let gopayAutoTriggerRunning = false;
let gopayCheckoutWatcherActive = false;
let stopGopayOTPAutoCapture = null;
let checkoutSubmitWatcherId = 0;
let gopayMidtransRetryNotBefore = 0;
let gopayMidtransCooldownNoticeKey = "";
const gopayAutoTriggeredCheckoutKeys = new Set();
const gptPlusSuccessRecordKeys = new Set();

const gopayStepItems = Array.from(document.querySelectorAll("[data-gopay-step]"));
const gopayStepTexts = {
  linking:  document.querySelector("#gopayLinkingText"),
  reference: document.querySelector("#gopayReferenceText"),
  consent:  document.querySelector("#gopayConsentText"),
  otp:      document.querySelector("#gopayOTPText"),
  pin:      document.querySelector("#gopayPINText"),
  success:  document.querySelector("#gopaySuccessText"),
};
const gopayVoiceStepLabels = {
  linking: "GoPay 绑定",
  reference: "引用验证",
  consent: "OTP 触发",
  otp: "OTP 验证",
  pin: "PIN 验证",
  success: "支付结果",
};
const gopayVoiceStateLabels = {
  active: "正在处理",
  done: "已完成",
  warning: "需要注意",
  error: "发生错误",
};
let gopayStepVoiceState = {};

function announceGopayStep(step, state, text) {
  if (!step || !state || state === "idle") return;
  var stepLabel = gopayVoiceStepLabels[step] || step;
  var stateLabel = gopayVoiceStateLabels[state] || state;
  var detail = normalizeVoicePromptMessage(text || "");
  if (!detail || detail === "等待提交") return;
  var key = "gopay_step_" + step + "_" + state + "_" + detail;
  if (gopayStepVoiceState[step] === key) return;
  gopayStepVoiceState[step] = key;
  announceLuckMailScene(key, stepLabel + stateLabel + "，" + detail, state === "error" || state === "warning" || state === "done");
}

function normalizeGopayPINValue(value) {
  return String(value || "").replace(/\D/g, "").slice(0, 6);
}

function currentGopayPINValue() {
  return normalizeGopayPINValue(gopayPINCodeInput?.value || "");
}

// GoPay 表单记忆功能
(function initGopayFormMemory() {
  var GPM = "gopay_form_";
  try {
    var retryDefaultMigrationKey = GPM + "aggressive_retry_default_v2";
    if (gopayAggressiveRetryInput && localStorage.getItem(retryDefaultMigrationKey) !== "1") {
      localStorage.setItem(GPM + "aggressive_retry", "0");
      localStorage.setItem(retryDefaultMigrationKey, "1");
    }
  } catch (_err) {}
  var fields = [
    { el: gopayAccountId,    key: "account_id" },
    { el: gopayCountryCode,  key: "country_code" },
    { el: gopayPhone,        key: "phone_number" },
    { el: gopayOTPChannel,   key: "otp_channel" },
    { el: gopayPINCodeInput, key: "pin_code" },
    { el: gopayAggressiveRetryInput, key: "aggressive_retry", type: "checkbox", defaultValue: "0" },
  ];

  fields.forEach(function (f) {
    if (!f.el) return;
    // 恢复上次值
    var saved = localStorage.getItem(GPM + f.key);
    if (f.type === "checkbox") {
      if (saved !== null && saved !== undefined) {
        f.el.checked = saved === "1";
      } else if (f.defaultValue !== undefined) {
        f.el.checked = f.defaultValue === "1";
      }
    } else if (saved !== null && saved !== undefined) {
      f.el.value = f.key === "pin_code" ? normalizeGopayPINValue(saved) : saved;
    }
    // 监听变化并保存
    var persistField = function () {
      if (f.key === "pin_code") {
        var normalizedPIN = normalizeGopayPINValue(f.el.value);
        if (f.el.value !== normalizedPIN) f.el.value = normalizedPIN;
        localStorage.setItem(GPM + f.key, normalizedPIN);
        return;
      }
      localStorage.setItem(GPM + f.key, f.type === "checkbox" ? (f.el.checked ? "1" : "0") : f.el.value);
    };
    f.el.addEventListener("input", persistField);
    f.el.addEventListener("change", persistField);
  });
})();

function setGopayStep(step, state, text) {
  const item = gopayStepItems.find(function (el) { return el.dataset.gopayStep === step; });
  if (!item) return;
  item.classList.remove("active", "done", "error", "warning");
  if (state) item.classList.add(state);
  const target = gopayStepTexts[step];
  if (target) setText(target, text || "-");
  announceGopayStep(step, state, text || "");
}

function resetGopaySteps() {
  gopayStepVoiceState = {};
  ["linking","reference","consent","otp","pin","success"].forEach(function (s) {
    setGopayStep(s, "", "等待提交");
  });
  if (gopayStepFlow) gopayStepFlow.hidden = false;
  if (gopayResult) gopayResult.hidden = true;
  if (gopayOutput) setText(gopayOutput, "{}");
  if (gopayBadge) { setText(gopayBadge, "运行中"); gopayBadge.className = "badge"; }
  if (gopayLinkStatus) setText(gopayLinkStatus, "-");
  if (gopayRefID) setText(gopayRefID, "-");
  if (gopayLatency) setText(gopayLatency, "-");
}

function renderGopayStageResults(stages) {
  if (!Array.isArray(stages)) return;
  var stageMap = {};
  stages.forEach(function (s) { if (s.name) stageMap[s.name] = s; });

  // force-link-api
  if (stageMap["force-link-api"]) {
    setGopayStep("linking", stageMap["force-link-api"].ok ? "done" : "error",
      stageMap["force-link-api"].message || (
        stageMap["force-link-api"].ok
          ? "已获取 reference_id"
          : (stageMap["force-link-api"].error || "请求失败")
      ));
  }

  // validate-reference
  if (stageMap["validate-reference"]) {
    setGopayStep("reference", stageMap["validate-reference"].ok ? "done" : "error",
      stageMap["validate-reference"].message || (stageMap["validate-reference"].ok ? "引用有效" : (stageMap["validate-reference"].error || "验证失败")));
  }

  // user-consent
  if (stageMap["user-consent"]) {
    setGopayStep("consent", stageMap["user-consent"].ok ? "done" : "error",
      stageMap["user-consent"].message || (stageMap["user-consent"].ok ? "OTP 已发送" : (stageMap["user-consent"].error || "失败")));
  }

  // otp-enum
  if (stageMap["otp-enum"]) {
    if (stageMap["otp-enum"].message) {
      setGopayStep("otp", "done", stageMap["otp-enum"].message);
    } else if (stageMap["otp-enum"].ok) {
      setGopayStep("otp", "done", "OTP 通过 (" + (stageMap["otp-enum"].otp || "?") + ")");
    } else if (stageMap["otp-enum"].tried && stageMap["otp-enum"].tried.length > 0) {
      setGopayStep("otp", "warning", "自动尝试 " + stageMap["otp-enum"].tried.length + " 个码均失败");
    } else {
      setGopayStep("otp", "error", stageMap["otp-enum"].error || "失败");
    }
  }

  // pin-enum
  if (stageMap["pin-enum"]) {
    if (stageMap["pin-enum"].message) {
      setGopayStep("pin", "done", stageMap["pin-enum"].message);
    } else if (stageMap["pin-enum"].ok) {
      setGopayStep("pin", "done", "PIN 已通过");
    } else if (stageMap["pin-enum"].tried && stageMap["pin-enum"].tried.length > 0) {
      setGopayStep("pin", "warning", "自动尝试 " + stageMap["pin-enum"].tried.length + " 个 PIN 均失败");
    } else {
      setGopayStep("pin", "error", stageMap["pin-enum"].error || "失败");
    }
  }

  // validate-pin
  if (stageMap["validate-pin"]) {
    setGopayStep("success", stageMap["validate-pin"].ok ? "done" : "error",
      stageMap["validate-pin"].message || (stageMap["validate-pin"].ok ? "GoPay 绑定成功" : (stageMap["validate-pin"].error || "失败")));
  }
}

function releaseGopayAutomationAfterTerminal() {
  gopayAutoTriggerRunning = false;
  gopayPaymentFlowRunning = false;
  gopayCheckoutWatcherActive = false;
  if (gopayLinkBtn) gopayLinkBtn.disabled = false;
  if (gopayMonitorBtn) gopayMonitorBtn.disabled = false;
}

function nextGopayPollDelayMs(data, result, unchangedPolls) {
  data = data || {};
  result = result || {};
  var pageStage = String(data.stage || result.page_stage || "").trim();
  var pinStage = String(data.pin_stage || result.pin_stage || "").trim();
  var balanceState = String(data.balance_state || result.balance_state || "").trim();
  var insufficientBalance = balanceState === "insufficient" || pageStage === "gopay_insufficient_balance";
  var autoActionStage = String(data.auto_action_stage || result.auto_action_stage || "").trim();
  var payNowBlockReason = String(data.pay_now_block_reason || result.pay_now_block_reason || "").trim();
  var hasPinField = !!(data.has_pin_field ?? result.has_pin_field);
  var hasOTPField = !!(data.has_otp_field ?? result.has_otp_field ?? result.hasOTPField);
  var payNowButtonDetected = !!(data.pay_now_button_detected ?? result.pay_now_button_detected);
  var payNowAttemptLimitReached = !!(data.pay_now_attempt_limit_reached ?? result.pay_now_attempt_limit_reached) || /attempt_limit/i.test(payNowBlockReason);
  var paymentCompleted = !!(data.payment_completed ?? result.payment_completed);
  var paymentExpired = !!(data.payment_expired ?? result.payment_expired);
  var paymentFailed = !!(data.payment_failed ?? result.payment_failed);
  var stableCount = Math.max(0, Number(unchangedPolls || 0));

  if (paymentCompleted || paymentExpired || paymentFailed) return 0;
  if (pageStage === "pin_entry_payment" || pageStage === "pin_entry_binding" || pinStage || hasPinField) return 550;
  if (pageStage === "otp_entry" || hasOTPField) return 900;
  if (["pay_now_post_click_wait", "pay_now_trusted_click"].includes(pageStage) || autoActionStage === "pay_now_trusted_click") return 650;
  if (insufficientBalance) return Math.min(7000, 3000 + stableCount * 600);
  if (payNowButtonDetected && !payNowAttemptLimitReached) return 800;
  if (balanceState === "rp0" || pageStage === "balance_wait_rp0") return Math.min(5000, 2400 + stableCount * 500);
  if (payNowAttemptLimitReached || payNowBlockReason === "synthetic_click_attempt_limit") return Math.min(5000, 2600 + stableCount * 400);
  if (stableCount >= 8) return 4000;
  if (stableCount >= 3) return 2500;
  return 1500;
}

function startGopayOTPAutoCapture(options) {
  if (typeof stopGopayOTPAutoCapture === "function") {
    stopGopayOTPAutoCapture();
  }
  options = options || {};
  var captureContext = {
    target_id: (options.target_id || options.targetID || "").trim(),
    target_url: (options.target_url || options.targetURL || "").trim(),
    account_id: (options.account_id || options.accountID || "").trim(),
    checkout_url: (options.checkout_url || options.checkoutURL || "").trim(),
  };
  var stop = false;
  var attempts = 0;
  var maxAttempts = 120;
  var pollDelayMs = 1500;
  var lastStageKey = "";
  var unchangedPolls = 0;
  var payNowStalledNoticeShown = false;
  var insufficientBalanceNoticeShown = false;

  var poll = async function () {
    if (stop || attempts >= maxAttempts) return;
    attempts++;

    try {
      var payload = {
        otp: (gopayOTPInput?.value || "").trim(),
        pin: currentGopayPINValue(),
        trace_id: ensureAutomationTraceID("gopay"),
        poll_delay_ms: pollDelayMs,
      };
      Object.keys(captureContext).forEach(function (key) {
        if (captureContext[key]) payload[key] = captureContext[key];
      });
      var resp = await fetch("/api/gopay/cdp-otp", {
        method: "POST",
        headers: buildRequestHeaders(),
        body: JSON.stringify(payload),
      });
      var data = await resp.json();
      var result = data.result || {};
      var pageStage = (data.stage || result.page_stage || "").trim();
      var pinStage = (data.pin_stage || result.pin_stage || "").trim();
      var otpManualRequired = !!(data.otp_manual_required ?? result.otp_manual_required);
      var pinAutoFilled = !!(data.pin_auto_filled ?? result.pin_auto_filled);
      var pinAutoSubmitted = !!(data.pin_auto_submitted ?? result.pin_auto_submitted);
      var hasOTPField = !!(data.has_otp_field ?? result.has_otp_field ?? result.hasOTPField);
      var hasPinField = !!(data.has_pin_field ?? result.has_pin_field);
      var inputStrategy = (data.pin_input_strategy || result.pin_input_strategy || "").trim();
      var balanceState = (data.balance_state || result.balance_state || "").trim();
      var balanceAmount = data.balance_amount ?? result.balance_amount;
      var orderAmount = data.order_amount ?? result.order_amount;
      var balanceShortfall = data.balance_shortfall ?? result.balance_shortfall;
      var insufficientBalanceReason = (data.insufficient_balance_reason || result.insufficient_balance_reason || "").trim();
      var hubungkanAutoClicked = !!(data.hubungkan_auto_clicked ?? result.hubungkan_auto_clicked);
      var payNowAutoClicked = !!(data.pay_now_auto_clicked ?? result.pay_now_auto_clicked);
      var payNowButtonDetected = !!(data.pay_now_button_detected ?? result.pay_now_button_detected);
      var payNowTrustedClicked = !!(data.pay_now_trusted_clicked ?? result.pay_now_trusted_clicked);
      var payNowBlockReason = (data.pay_now_block_reason || result.pay_now_block_reason || "").trim();
      var payNowAttemptLimitReached = !!(data.pay_now_attempt_limit_reached ?? result.pay_now_attempt_limit_reached) || /attempt_limit/i.test(payNowBlockReason);
      var payNowPostClickElapsedMS = Number(data.pay_now_post_click_elapsed_ms ?? result.pay_now_post_click_elapsed_ms ?? 0) || 0;
      var autoActionPaused = !!(data.auto_action_paused ?? result.auto_action_paused);
      var autoActionStage = (data.auto_action_stage || result.auto_action_stage || "").trim();
      var pinAutoBlockReason = (data.pin_auto_block_reason || result.pin_auto_block_reason || "").trim();
      var pinAttemptCount = data.pin_attempt_count ?? result.pin_attempt_count ?? "";
      var paymentCompleted = !!(data.payment_completed ?? result.payment_completed);
      var paymentExpired = !!(data.payment_expired ?? result.payment_expired);
      var paymentFailed = !!(data.payment_failed ?? result.payment_failed) || pageStage === "gopay_payment_failed";
      var paymentFailureReason = (data.payment_failure_reason || result.payment_failure_reason || "").trim();
      var stageKey = [
        pageStage,
        pinStage,
        autoActionStage,
        paymentCompleted ? "payment-completed" : "",
        balanceState,
        orderAmount,
        paymentExpired ? "payment-expired" : "",
        paymentFailed ? "payment-failed" : "",
        paymentFailureReason,
        balanceAmount,
        balanceShortfall,
        insufficientBalanceReason,
        hubungkanAutoClicked ? "hubungkan-clicked" : "",
        payNowAutoClicked ? "pay-now-clicked" : "",
        payNowTrustedClicked ? "pay-now-trusted-clicked" : "",
        payNowButtonDetected ? "pay-now-detected" : "",
        payNowAttemptLimitReached ? "pay-now-attempt-limit" : "",
        payNowBlockReason,
        payNowPostClickElapsedMS,
        autoActionPaused ? "paused" : "",
        otpManualRequired ? "otp-manual" : "",
        pinAutoFilled ? "pin-filled" : "",
        pinAutoSubmitted ? "pin-submitted" : "",
        pinAutoBlockReason,
        pinAttemptCount,
        inputStrategy,
      ].join("|");
      var stageChanged = stageKey && stageKey !== lastStageKey;
      if (stageKey) {
        unchangedPolls = stageChanged ? 0 : unchangedPolls + 1;
      }
      pollDelayMs = nextGopayPollDelayMs(data, result, unchangedPolls) || pollDelayMs;

      if (paymentCompleted || pageStage === "gopay_complete") {
        setGopayStep("success", "done", "支付已完成");
        appendCheckoutWatcherEvent("gopay-complete", "检测到 GoPay 支付完成，开始保存 GPT Plus 成功记录");
        if (gopayBadge) {
          setText(gopayBadge, "支付完成");
          gopayBadge.className = "badge";
        }
        await saveGPTPlusSuccessRecord(Object.assign({}, data, { stage: "gopay_complete" }));
        stop = true;
        return;
      }
      if (paymentExpired || pageStage === "gopay_session_expired") {
        setGopayStep("success", "error", "GoPay 会话已超时，请关闭旧支付页并重新生成结账链路");
        appendCheckoutWatcherEvent("gopay-session-expired", "检测到 GoPay 页面提示 Yah, waktunya habis，当前支付会话已失效");
        showAutoFillNotice(
          "GoPay 支付页已过期",
          "已停止",
          "当前 Midtrans/GoPay 支付会话已经失效，继续点击旧页面不会完成扣款。",
          "请关闭旧支付页，重新生成支付链接后再打开新的 GoPay 支付页。",
          "error"
        );
        if (gopayBadge) {
          setText(gopayBadge, "已超时");
          gopayBadge.className = "badge error";
        }
        releaseGopayAutomationAfterTerminal();
        stopGopayOTPAutoCapture = null;
        stop = true;
        return;
      }
      if (paymentFailed) {
        setGopayStep("success", "error", "GoPay 支付页返回失败，请重新生成结账链路");
        appendCheckoutWatcherEvent("gopay-payment-failed", "检测到支付页失败态：" + (paymentFailureReason || pageStage || "payment_failed"));
        showAutoFillNotice(
          "GoPay 支付失败",
          "已停止",
          "支付页提示 Failed to complete payment，需要重新下单或更换支付方式。",
          "请关闭当前旧支付页，重新生成 OpenAI 结账链接后再触发 GoPay。",
          "error"
        );
        if (gopayBadge) {
          setText(gopayBadge, "支付失败");
          gopayBadge.className = "badge error";
        }
        releaseGopayAutomationAfterTerminal();
        stopGopayOTPAutoCapture = null;
        stop = true;
        return;
      }

      if (stageChanged) {
        lastStageKey = stageKey;
        if (pageStage === "gopay_consent_hubungkan" || hubungkanAutoClicked) {
          setGopayStep("consent", "done", "Hubungkan 已出现并自动确认");
          appendCheckoutWatcherEvent("gopay-hubungkan", "检测到 Hubungkan，已执行一次自动确认");
          if (gopayBadge) {
            setText(gopayBadge, "已确认");
            gopayBadge.className = "badge";
          }
        }
        if (pageStage === "gopay_insufficient_balance" || balanceState === "insufficient" || payNowBlockReason === "insufficient_balance") {
          setGopayStep("success", "warning", "GoPay 余额不足，已暂停 Pay now 自动点击");
          var amountText = orderAmount && balanceAmount ? "订单 Rp" + orderAmount + "，余额 Rp" + balanceAmount : "当前余额不足以覆盖订单金额";
          appendCheckoutWatcherEvent("gopay-insufficient-balance", "检测到 GoPay 余额不足：" + amountText + (balanceShortfall ? "，缺口 Rp" + balanceShortfall : ""));
          if (!insufficientBalanceNoticeShown) {
            insufficientBalanceNoticeShown = true;
            showAutoFillNotice(
              "GoPay 余额不足",
              "已暂停",
              "支付页提示余额不足，系统已停止自动点击 Pay now。",
              "请完成充值并在支付页点击 Refresh；余额满足订单金额后监听会继续判断下一步。",
              "warning"
            );
          }
          if (gopayBadge) {
            setText(gopayBadge, "余额不足");
            gopayBadge.className = "badge neutral";
          }
        } else if (pageStage === "balance_wait_rp0" || autoActionPaused || balanceState === "rp0") {
          setGopayStep("success", "warning", "余额 Rp0，已暂停自动操作并等待余额变化");
          appendCheckoutWatcherEvent("gopay-balance-wait", "检测到账户余额 Rp0，自动操作已暂停");
          if (gopayBadge) {
            setText(gopayBadge, "等余额");
            gopayBadge.className = "badge neutral";
          }
        } else if (pageStage === "pay_now_post_click_wait" && payNowBlockReason === "trusted_click_no_transition") {
          setGopayStep("success", "warning", "Pay now 已真实点击一次，停止重复点击并继续监听 PIN 页面");
          if (!payNowStalledNoticeShown) {
            payNowStalledNoticeShown = true;
            appendCheckoutWatcherEvent("gopay-pay-now-stalled", "Pay now 真实点击后 " + Math.round(payNowPostClickElapsedMS / 1000) + " 秒仍未进入 PIN/成功页；已停止重复点击，但继续被动监听 PIN 页面");
            showAutoFillNotice(
              "Pay now 未立即推进",
              "继续监听",
              "系统只执行了一次 CDP 真实鼠标点击，不会重复点击 Pay now。",
              "如果 GoPay PIN 页面延迟出现或你手动推进到 PIN 页面，自动 PIN 输入仍会继续工作。",
              "warning"
            );
          }
          if (gopayBadge) {
            setText(gopayBadge, "监听 PIN");
            gopayBadge.className = "badge neutral";
          }
        } else if (pageStage === "pay_now_post_click_wait") {
          setGopayStep("success", "active", "Pay now 已真实点击一次，正在等待支付确认页");
          appendCheckoutWatcherEvent("gopay-pay-now-wait-after-click", "Pay now 已真实点击一次，等待页面进入 PIN/成功页");
          if (gopayBadge) {
            setText(gopayBadge, "等待跳转");
            gopayBadge.className = "badge neutral";
          }
        } else if (payNowButtonDetected && payNowAttemptLimitReached) {
          setGopayStep("success", "warning", "Pay now 已达到自动点击上限，请在支付页手动确认或重新生成支付链接");
          appendCheckoutWatcherEvent("gopay-pay-now-limit", "Pay now 自动点击已达上限，停止重复点击；请手动确认支付页状态");
          if (gopayBadge) {
            setText(gopayBadge, "需手动");
            gopayBadge.className = "badge neutral";
          }
        } else if (pageStage === "pay_now_trusted_click" || pageStage === "pay_now_ready" || pageStage === "pay_now_rp1" || payNowTrustedClicked || payNowAutoClicked) {
          setGopayStep("success", "active", payNowTrustedClicked ? "检测到可用余额，已用真实点击触发 Pay now" : "检测到可用余额，已自动点击 Pay now");
          appendCheckoutWatcherEvent("gopay-pay-now", payNowTrustedClicked ? "检测到 Pay now，已通过 CDP 真实鼠标点击" : "检测到可用余额与 Pay now，已自动点击");
          if (gopayBadge) {
            setText(gopayBadge, "Pay now");
            gopayBadge.className = "badge";
          }
        } else if (pageStage === "pay_now_ready_observed" || payNowButtonDetected) {
          setGopayStep("success", "active", "检测到 Pay now，等待页面响应或下一次自动点击");
          appendCheckoutWatcherEvent("gopay-pay-now-wait", "检测到 Pay now 按钮" + (payNowBlockReason ? "（" + payNowBlockReason + "）" : ""));
        } else if (pageStage === "balance_ready_observed" || pageStage === "balance_rp1_observed" || balanceState === "rp1" || balanceState === "other") {
          setGopayStep("success", "active", "检测到 GoPay 可用余额，正在等待 Pay now 可点击");
          appendCheckoutWatcherEvent("gopay-pay-now-wait", "检测到 GoPay 可用余额，等待 Pay now 按钮可点击");
        }
        if (pageStage === "otp_entry" || hasOTPField || otpManualRequired) {
          setGopayStep("otp", "active", "检测到 OTP 页面，等待手动输入");
          appendCheckoutWatcherEvent("gopay-otp-manual", "已进入 OTP 页面，OTP 保持手动输入");
          if (gopayBadge) {
            setText(gopayBadge, "等 OTP");
            gopayBadge.className = "badge neutral";
          }
        }
        if (pageStage === "pin_entry_binding" || pageStage === "pin_entry_payment" || hasPinField) {
          var pinLabel = pinStage === "payment" ? "支付确认 PIN" : "绑定授权 PIN";
          var pinMessage = "检测到 " + pinLabel + " 页面";
          var pinState = "active";
          if (pinAutoSubmitted) {
            pinMessage = pinLabel + " 已自动填入并提交";
            pinState = "done";
          } else if (pinAutoFilled) {
            pinMessage = pinLabel + " 已自动填入，等待页面继续";
          } else if (pinAutoBlockReason === "cooldown") {
            pinMessage = pinLabel + " 自动输入冷却中，稍后会再次尝试";
          } else if (pinAutoBlockReason === "attempt_limit") {
            pinMessage = pinLabel + " 已多次自动尝试，等待人工确认";
            pinState = "warning";
          } else if (!currentGopayPINValue()) {
            pinMessage = "检测到 " + pinLabel + " 页面，但面板里还没有可复用的 PIN";
            pinState = "warning";
          }
          setGopayStep("pin", pinState, pinMessage);
          appendCheckoutWatcherEvent(
            pageStage === "pin_entry_payment" ? "gopay-payment-pin" : "gopay-binding-pin",
            pinMessage + (inputStrategy ? "（" + inputStrategy + "）" : "")
          );
          if (gopayBadge) {
            setText(gopayBadge, pinStage === "payment" ? "支付 PIN" : "绑定 PIN");
            gopayBadge.className = pinAutoSubmitted ? "badge" : "badge neutral";
          }
        }
      }
    } catch (error) {
      console.error("otp auto capture failed:", error);
    }

    if (!stop && attempts < maxAttempts) {
      window.setTimeout(poll, pollDelayMs);
    }
  };

  window.setTimeout(poll, 800);
  stopGopayOTPAutoCapture = function () { stop = true; };
  return stopGopayOTPAutoCapture;
}

function checkoutAutoTriggerKey(rawURL) {
  var value = (rawURL || "").trim();
  if (!value) return "";
  try {
    var parsed = new URL(value);
    var host = parsed.hostname.toLowerCase();
    var path = parsed.pathname;
    var match = path.match(/\/(?:c\/pay|pay|checkout\/openai_llc)\/(cs_[^/?#]+)/i);
    if (!match) return "";
    if (host !== "pay.openai.com" && host !== "checkout.stripe.com" && host !== "chatgpt.com") return "";
    return match[1];
  } catch (_err) {
    return "";
  }
}

function checkoutURLIsManagedCheckout(currentURL) {
  return !!checkoutAutoTriggerKey(currentURL);
}

function checkoutAutoFillProbeText(data) {
  var chunks = [];
  ["probe_after_gopay", "probe_fallback", "probe"].forEach(function (key) {
    var text = data?.[key]?.text;
    if (typeof text === "string" && text.trim()) {
      chunks.push(text.trim());
    }
  });
  return chunks.join(" ");
}

function checkoutURLIndicatesSubmitted(currentURL, checkoutKey) {
  var value = (currentURL || "").trim();
  if (!value) return false;
  try {
    var parsed = new URL(value);
    var host = parsed.hostname.toLowerCase();
    var path = parsed.pathname.toLowerCase();
    if (host.includes("midtrans") || host.includes("gopay")) return true;
    if (path.includes("/checkout/verify") || path.includes("/snap/") || path.includes("/redirection/")) return true;
    var currentKey = checkoutAutoTriggerKey(value);
    if (checkoutKey && currentKey) return currentKey !== checkoutKey;
    if (checkoutURLIsManagedCheckout(value)) return false;
    if (checkoutKey && !currentKey && host !== "pay.openai.com" && host !== "checkout.stripe.com" && host !== "chatgpt.com") return true;
    return false;
  } catch (_err) {
    var lower = value.toLowerCase();
    return lower.includes("midtrans.com") || lower.includes("app.gopay") || lower.includes("/snap/");
  }
}

function midtransLinkingAccountID(currentURL) {
  var value = (currentURL || "").trim();
  if (!value) return "";
  try {
    var parsed = new URL(value);
    if (!parsed.hostname.toLowerCase().includes("midtrans.com")) return "";
    if (!parsed.hash.toLowerCase().includes("gopay-tokenization/linking")) return "";
    var match = parsed.pathname.match(/\/snap\/v4\/redirection\/([^/?#]+)/i);
    return match ? decodeURIComponent(match[1]) : "";
  } catch (_err) {
    return "";
  }
}

function midtransRedirectionAccountID(currentURL) {
  var value = (currentURL || "").trim();
  if (!value) return "";
  try {
    var parsed = new URL(value);
    if (!parsed.hostname.toLowerCase().includes("midtrans.com")) return "";
    var match = parsed.pathname.match(/\/snap\/v4\/redirection\/([^/?#]+)/i);
    return match ? decodeURIComponent(match[1]) : "";
  } catch (_err) {
    return "";
  }
}

const checkoutWatcherVoiceEventMessages = {
  "checkout-submitted": "检测到订阅付款页面已提交，正在等待 GoPay 支付页面。",
  "auto-trigger-ready": "GoPay 支付页面已识别，正在自动填写绑定信息。",
  "midtrans-linking-fill": "GoPay 绑定信息已填写并提交。",
  "midtrans-phone-binding-required": "GoPay 手机号不可用，需要先解除手机绑定。",
  "gopay-hubungkan": "GoPay 授权确认已自动处理。",
  "gopay-insufficient-balance": "GoPay 余额不足，已暂停自动点击 Pay now。请充值后点击 Refresh。",
  "gopay-balance-wait": "GoPay 余额为零，自动操作已暂停，等待余额变化。",
  "gopay-pay-now": "检测到 Pay now，已执行一次真实点击。",
  "gopay-pay-now-wait-after-click": "Pay now 已点击，正在等待进入 PIN 或成功页面。",
  "gopay-pay-now-stalled": "Pay now 点击后页面没有推进，已停止重复点击并继续监听 PIN 页面。",
  "gopay-pay-now-limit": "Pay now 已达到自动点击上限，请人工确认支付页状态。",
  "gopay-otp-manual": "已进入 GoPay OTP 页面，请手动输入 OTP 验证码。",
  "gopay-binding-pin": "已检测到绑定授权 PIN 页面，正在尝试自动输入 PIN。",
  "gopay-payment-pin": "已检测到支付确认 PIN 页面，正在尝试自动输入 PIN。",
  "gopay-complete": "GoPay 支付已完成，正在保存成功记录。",
  "gopay-session-expired": "GoPay 支付会话已超时，请重新生成支付链接。",
  "gopay-payment-failed": "GoPay 支付页返回失败，请重新生成结账链路。",
  "gptpls-record": "GPT Plus 成功记录已保存。",
  "gptpls-record-error": "GPT Plus 成功记录保存失败。",
  "auto-trigger-timeout": "等待订阅提交超时，未自动触发 GoPay。",
  "auto-trigger-error": "自动触发监控出现错误。",
};

function announceCheckoutWatcherVoice(method, summary) {
  var eventKey = String(method || "").trim();
  if (!eventKey) return;
  var message = checkoutWatcherVoiceEventMessages[eventKey] || "";
  if (!message && /^(gopay-|midtrans-|auto-trigger-|gptpls-record)/.test(eventKey)) {
    message = summary || "";
  }
  if (!message) return;
  announceLuckMailScene("watcher_" + eventKey + "_" + normalizeVoicePromptMessage(message), message, false);
}

function appendCheckoutWatcherEvent(method, summary) {
  if (gopayMonitor) {
    gopayMonitor.hidden = false;
  }
  if (typeof appendMonitorEvent === "function") {
    appendMonitorEvent({ domain: "Log", method: method, summary: summary, ts: Date.now() });
  }
  announceCheckoutWatcherVoice(method, summary);
}

function prepareCheckoutWatcherMonitor() {
  if (gopayMonitor) {
    gopayMonitor.hidden = false;
  }
  if (!gopayMonitorRunning && !sessionMonitorAbortController) {
    clearMonitor();
  }
  if (monitorBadge) {
    monitorBadge.textContent = "等待订阅";
    monitorBadge.className = "badge";
  }
}

async function checkGopayAutoTriggerReady(payload) {
  payload = Object.assign({ trace_id: ensureAutomationTraceID("gopay") }, payload || {});
  var response = await fetch("/api/gopay/auto-trigger-check", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify(payload),
  });
  var data = await response.json();
  if (!response.ok) {
    throw new Error(data.error || "GoPay 自动触发检查失败");
  }
  return data;
}

async function fillCurrentMidtransLinkingPage(targetURL, checkoutURL) {
  var countryCode = (gopayCountryCode?.value || "86").trim();
  var phoneNumber = (gopayPhone?.value || "18120322232").trim();
  var aggressiveRetry = !!gopayAggressiveRetryInput?.checked;
  var debugNetworkOnce = false;
  try {
    debugNetworkOnce = window.localStorage?.getItem("gopay_debug_network_once") === "1";
    if (debugNetworkOnce) window.localStorage?.removeItem("gopay_debug_network_once");
  } catch (_err) {
    debugNetworkOnce = false;
  }
  var response = await fetch("/api/gopay/midtrans-linking-fill", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify({
      target_url: targetURL,
      checkout_url: checkoutURL || "",
      country_code: countryCode,
      phone_number: phoneNumber,
      aggressive_retry: aggressiveRetry,
      debug_network: debugNetworkOnce,
      trace_id: ensureAutomationTraceID("gopay"),
    }),
  });
  var data = await response.json();
  if (!response.ok) {
    throw new Error(data.error || "Midtrans GoPay 页面填充失败");
  }
  return data;
}

function midtransPhoneBindingRequired(data) {
  var result = data?.browser_result || data?.result || {};
  var text = [
    data?.stage,
    data?.phone_binding_error_text,
    data?.error,
    result?.stage,
    result?.phone_binding_error_text,
    result?.page_text_snippet,
  ].filter(Boolean).join(" ").toLowerCase();
  return !!(
    data?.stage === "midtrans_phone_binding_required" ||
    data?.phone_binding_error ||
    result?.phone_binding_error ||
    text.includes("please use another phone number") ||
    text.includes("use another phone number") ||
    text.includes("gunakan nomor telepon lain") ||
    text.includes("pakai nomor telepon lain")
  );
}

function startCheckoutSubmitWatcher(autoFillData) {
  var checkoutURL = autoFillData?.current_url || autoFillData?.expected_url || latestOpenedCheckoutURL;
  var checkoutKey = checkoutAutoTriggerKey(checkoutURL);
  var pageText = checkoutAutoFillProbeText(autoFillData);

  if (!checkoutKey) {
    appendCheckoutWatcherEvent("auto-trigger-skip", "未识别到 pay.openai.com checkout URL，跳过自动触发监控");
    return;
  }
  if (gopayAutoTriggeredCheckoutKeys.has(checkoutKey)) {
    appendCheckoutWatcherEvent("auto-trigger-skip", "该 checkout 已触发过 GoPay 一键绑定，跳过重复监控");
    return;
  }

  checkoutSubmitWatcherId += 1;
  var watcherId = checkoutSubmitWatcherId;
  var attempts = 0;
  var maxAttempts = 45;
  var lastCurrentURL = "";
  var watcherButtonText = gopayLinkBtn ? gopayLinkBtn.textContent : "";
  var finishWatcher = function () {
    if (watcherId !== checkoutSubmitWatcherId) return;
    gopayCheckoutWatcherActive = false;
    if (gopayLinkBtn && !gopayPaymentFlowRunning) {
      gopayLinkBtn.disabled = false;
      setText(gopayLinkBtn, watcherButtonText || "GoPay 一键绑定");
      gopayLinkBtn.removeAttribute("title");
    }
  };

  prepareCheckoutWatcherMonitor();
  gopayCheckoutWatcherActive = true;
  if (gopayLinkBtn) {
    gopayLinkBtn.disabled = true;
    gopayLinkBtn.title = "自动监控已接管本次 checkout，请等待 Midtrans linking 页面出现。";
    setText(gopayLinkBtn, "自动监控中...");
  }
  appendCheckoutWatcherEvent("auto-trigger-watch", "已开始等待支付页订阅提交：" + checkoutKey);

  void checkGopayAutoTriggerReady({
    source: "checkout-auto-fill-watch",
    checkout_url: checkoutURL,
    page_text: pageText,
    submitted: false,
  }).then(function (decision) {
    appendCheckoutWatcherEvent("auto-trigger-wait", "自动触发判定：" + (decision.reason || "waiting"));
  }).catch(function (error) {
    appendCheckoutWatcherEvent("auto-trigger-check-error", error.message || "自动触发预检查失败");
  });

  var poll = async function () {
    if (watcherId !== checkoutSubmitWatcherId || gopayAutoTriggeredCheckoutKeys.has(checkoutKey)) {
      finishWatcher();
      return;
    }
    attempts += 1;

    try {
      var response = await fetch("/api/checkout/resolve-target", {
        method: "POST",
        headers: buildRequestHeaders(),
        body: JSON.stringify({
          opened_url: checkoutURL,
          target_id: latestOpenedCheckoutTargetID,
          trace_id: ensureAutomationTraceID("checkout"),
        }),
      });
      var data = await response.json();
      if (data?.target?.id || data?.target_id) {
        latestOpenedCheckoutTargetID = data.target?.id || data.target_id;
      }
      if (data?.current_url) {
        latestOpenedCheckoutURL = data.current_url;
      }
      var currentURL = data.current_url || data.target?.url || "";
      var candidateURLs = Array.isArray(data.candidate_urls) ? data.candidate_urls : [];
      var submittedCandidateURL = candidateURLs.find(function (url) {
        return checkoutURLIndicatesSubmitted(url, checkoutKey);
      }) || "";
      if (!currentURL && submittedCandidateURL) {
        currentURL = submittedCandidateURL;
      }
      var linkingURL = midtransRedirectionAccountID(currentURL) ? currentURL : "";
      if (!linkingURL) {
        linkingURL = candidateURLs.find(function (url) {
          return !!midtransRedirectionAccountID(url);
        }) || "";
      }
      var hasMidtransCandidate = !!linkingURL;
      if (currentURL && currentURL !== lastCurrentURL) {
        lastCurrentURL = currentURL;
        appendCheckoutWatcherEvent("checkout-url", currentURL);
      } else if (hasMidtransCandidate && linkingURL !== lastCurrentURL) {
        lastCurrentURL = linkingURL;
        appendCheckoutWatcherEvent("checkout-url", linkingURL);
      }

      if (response.ok && (data.ok || submittedCandidateURL || hasMidtransCandidate) && (checkoutURLIndicatesSubmitted(currentURL, checkoutKey) || hasMidtransCandidate)) {
        appendCheckoutWatcherEvent("checkout-submitted", hasMidtransCandidate ? "检测到 Midtrans GoPay redirection 页面，开始 GoPay 自动触发检查" : "检测到支付页已提交，开始 GoPay 自动触发检查");
        var decision = await checkGopayAutoTriggerReady({
          source: "checkout-submit-watch",
          checkout_url: checkoutURL,
          page_text: pageText,
          submitted: true,
        });

        if (decision.ready) {
          if (monitorBadge) {
            monitorBadge.textContent = "自动触发";
            monitorBadge.className = "badge";
          }
          if (!linkingURL) {
            appendCheckoutWatcherEvent("auto-trigger-wait", "支付页已提交，继续等待 Midtrans GoPay redirection 页面");
            if (monitorBadge) {
              monitorBadge.textContent = "等 Midtrans";
              monitorBadge.className = "badge neutral";
            }
          } else if (!gopayAutoTriggerRunning) {
            var cooldownRemainingMs = gopayMidtransRetryNotBefore - Date.now();
            if (cooldownRemainingMs > 0) {
              var cooldownSeconds = Math.ceil(cooldownRemainingMs / 1000);
              var cooldownKey = linkingURL + "|" + Math.ceil(gopayMidtransRetryNotBefore / 5000);
              if (cooldownKey !== gopayMidtransCooldownNoticeKey) {
                gopayMidtransCooldownNoticeKey = cooldownKey;
                appendCheckoutWatcherEvent("midtrans-linking-cooldown", "Midtrans 页面冷却中，约 " + cooldownSeconds + " 秒后再尝试");
              }
              if (monitorBadge) {
                monitorBadge.textContent = "冷却中";
                monitorBadge.className = "badge neutral";
              }
            } else {
            appendCheckoutWatcherEvent("auto-trigger-ready", "条件满足，填写当前 Midtrans GoPay 页面：" + midtransRedirectionAccountID(linkingURL));
            gopayAutoTriggerRunning = true;
            try {
              var fillResult = await fillCurrentMidtransLinkingPage(linkingURL, checkoutURL);
              if (gopayOutput) setText(gopayOutput, JSON.stringify(fillResult, null, 2));
              if (fillResult.ok) {
                gopayMidtransRetryNotBefore = 0;
                gopayMidtransCooldownNoticeKey = "";
                gopayAutoTriggeredCheckoutKeys.add(checkoutKey);
                appendCheckoutWatcherEvent("midtrans-linking-fill", "已填写 +" + (fillResult.country_code || "86") + " / " + (fillResult.phone_number || "18120322232") + " 并提交 Link and pay");
                if (monitorBadge) {
                  monitorBadge.textContent = "已提交绑定";
                  monitorBadge.className = "badge";
                }
                startGopayOTPAutoCapture({
                  target_id: fillResult.cdp_target_id || "",
                  target_url: fillResult.cdp_target_url || fillResult.target_url || linkingURL,
                  account_id: fillResult.account_id || midtransRedirectionAccountID(linkingURL),
                  checkout_url: checkoutURL,
                });
                finishWatcher();
                return;
              } else {
                if (midtransPhoneBindingRequired(fillResult)) {
                  var phoneBindingPrompt = fillResult.voice_prompt || fillResult.browser_result?.voice_prompt || "请解除手机绑定";
                  var phoneBindingText = fillResult.phone_binding_error_text || fillResult.browser_result?.phone_binding_error_text || "Please use another phone number";
                  appendCheckoutWatcherEvent("midtrans-phone-binding-required", "检测到手机号不可用：" + phoneBindingText);
                  setGopayStep("linking", "error", "手机号已绑定或不可用，请先解除手机绑定");
                  if (gopayBadge) {
                    setText(gopayBadge, "解绑手机");
                    gopayBadge.className = "badge error";
                  }
                  if (monitorBadge) {
                    monitorBadge.textContent = "需解绑";
                    monitorBadge.className = "badge error";
                  }
                  announceLuckMailScene("gopay_phone_binding_required_" + (fillResult.phone_number || ""), phoneBindingPrompt);
                  finishWatcher();
                  return;
                }
                var retryHint = fillResult.aggressive_retry ? "（保守恢复已开启）" : "";
                var fillStage = fillResult.stage || fillResult.error || "unknown";
                var retryAfterMs = Number(fillResult.retry_after_ms || fillResult.cooldown_ms || 0);
                if (!Number.isFinite(retryAfterMs)) retryAfterMs = 0;
                if (!retryAfterMs && fillStage === "midtrans_linking_rate_limited") retryAfterMs = 90000;
                if (!retryAfterMs && (fillStage === "midtrans_linking_blank_shell" || fillStage === "midtrans_linking_loading_stuck")) retryAfterMs = 30000;
                if (retryAfterMs > 0) {
                  retryAfterMs = Math.min(Math.max(retryAfterMs, 5000), 120000);
                  gopayMidtransRetryNotBefore = Date.now() + retryAfterMs;
                  gopayMidtransCooldownNoticeKey = "";
                  appendCheckoutWatcherEvent("midtrans-linking-cooldown", "页面处于 " + fillStage + "，冷却 " + Math.ceil(retryAfterMs / 1000) + " 秒后再尝试");
                  if (monitorBadge) {
                    monitorBadge.textContent = fillStage === "midtrans_linking_rate_limited" ? "已限流" : "加载中";
                    monitorBadge.className = "badge neutral";
                  }
                }
                appendCheckoutWatcherEvent("midtrans-linking-fill", "页面填充未完成：" + fillStage + retryHint);
                if (monitorBadge) {
                  monitorBadge.textContent = retryAfterMs > 0 ? "冷却中" : "继续等待";
                  monitorBadge.className = "badge neutral";
                }
              }
            } finally {
              gopayAutoTriggerRunning = false;
            }
            }
          }
          if (gopayAutoTriggeredCheckoutKeys.has(checkoutKey)) return;
        }

        if (!decision.ready) {
          appendCheckoutWatcherEvent("auto-trigger-skip", "自动触发检查未通过：" + (decision.reason || "unknown"));
          if (monitorBadge) {
            monitorBadge.textContent = "未触发";
            monitorBadge.className = "badge neutral";
          }
          finishWatcher();
          return;
        }
      }
    } catch (error) {
      appendCheckoutWatcherEvent("auto-trigger-error", error.message || "自动触发监控失败");
    }

    if (attempts < maxAttempts) {
      window.setTimeout(poll, 2000);
    } else {
      appendCheckoutWatcherEvent("auto-trigger-timeout", "等待订阅提交超时，未自动触发 GoPay 一键绑定");
      if (monitorBadge) {
        monitorBadge.textContent = "等待超时";
        monitorBadge.className = "badge neutral";
      }
      finishWatcher();
    }
  };

  window.setTimeout(poll, 1500);
}

async function runGopayFullLinkPayment(options) {
  options = options || {};
  var triggerSource = options.triggerSource || "manual";
  if (gopayPaymentFlowRunning) {
    showAutoFillNotice("GoPay 付款进行中", "提示", "当前一键绑定付款流程仍在执行，请等待本次流程完成。", "", "");
    return null;
  }

  var countryCode = (gopayCountryCode?.value || "86").trim();
  var phoneNumber = (gopayPhone?.value || "18120322232").trim();
  var otpChannel = gopayOTPChannel?.value || "whatsapp";
  var otpCode = (gopayOTPInput?.value || "").trim();
  var pinCode = currentGopayPINValue();
  var accessToken = extractAccessToken(fields.token?.value || "");

  if (!phoneNumber) {
    setWorkflowButtonState(gopayLinkBtn, "error");
    showAutoFillNotice("GoPay 绑定失败", "错误", "请输入手机号。", "", "error");
    return;
  }

  resetGopaySteps();
  gopayPaymentFlowRunning = true;
  if (gopayLinkBtn) gopayLinkBtn.disabled = true;
  setWorkflowButtonState(gopayLinkBtn, "running");
  if (gopayMonitorBtn) gopayMonitorBtn.disabled = true;
  var origText = gopayLinkBtn ? gopayLinkBtn.textContent : "";
  setText(gopayLinkBtn, triggerSource === "auto-trigger" ? "自动触发中..." : "提取 Session 中...");

  var startedAt = performance.now();

  try {
    if (!accessToken) {
      await fetchLatestSessionJSON();
      accessToken = extractAccessToken(fields.token?.value || "");
    }
    if (!accessToken) {
      throw new Error("无法自动提取最新 Session JSON 或 access token");
    }

    setText(gopayLinkBtn, "创建新链路中...");
    setGopayStep("linking", "active", "正在提取最新 Session 并生成新的 Plus 结账链路...");
    startGopayOTPAutoCapture({ checkout_url: latestOpenedCheckoutURL || "" });

    var payload = {
      access_token: accessToken,
      country_code: countryCode,
      phone_number: phoneNumber,
      otp_channel: otpChannel,
      trace_id: ensureAutomationTraceID("gopay"),
    };
    if (otpCode) payload.otp = otpCode;
    if (pinCode) payload.pin = pinCode;

    var resp = await fetch("/api/gopay/full-link", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(payload),
    });
    var data = await resp.json();
    var elapsed = Math.round(performance.now() - startedAt);

    if (data.account_id && gopayAccountId) {
      gopayAccountId.value = data.account_id;
      localStorage.setItem("gopay_form_account_id", data.account_id);
    }

    if (gopayStepFlow) gopayStepFlow.hidden = false;
    if (gopayResult) gopayResult.hidden = false;
    if (gopayLatency) setText(gopayLatency, elapsed + "ms");
    if (gopayLinkStatus) {
      var okText = "已绑定";
      if (data.stage === "gopay_complete") {
        okText = "支付已完成";
      } else if (data.reused_existing) {
        okText = "已复用已绑定账号";
      }
      setText(gopayLinkStatus, data.ok ? okText : "失败：" + (data.stage || "未知阶段"));
    }
    if (gopayRefID) setText(gopayRefID, (data.reference_id || data.account_id || "-").slice(0, 36));
    if (gopayOutput) setText(gopayOutput, JSON.stringify(data, null, 2));

    if (data.stages) {
      renderGopayStageResults(data.stages);
    } else {
      var manualStages = [];
      if (data.stage === "linking_success" || data.stage === "gopay_complete") {
        manualStages = [
          { name: "force-link-api", ok: true },
          { name: "validate-reference", ok: true },
          { name: "user-consent", ok: true },
          { name: "otp-enum", ok: true, otp: data.summary?.otp },
          { name: "pin-enum", ok: true, pin: data.summary?.pin || data.summary?.payment_pin },
          { name: "validate-pin", ok: true },
        ];
      } else if (data.stage) {
        var stageToErrorMap = {
          checkout_api_failed: "linking",
          no_account_id: "linking",
          force_link_failed: "linking",
          validate_reference_failed: "reference",
          user_consent_failed: "consent",
          otp_all_failed: "otp",
          pin_all_failed: "pin",
          payment_pin_all_failed: "pin",
          gopay_payment_failed: "pin",
          validate_pin_failed: "success",
        };
        var uiStage = stageToErrorMap[data.stage];
        if (uiStage) {
          manualStages.push({ name: uiStage === "linking" ? "force-link-api" : uiStage, ok: false, error: data.error || data.stage });
        }
      }
      if (manualStages.length > 0) renderGopayStageResults(manualStages);
    }

    if (data.ok) {
      var successText = "GoPay 绑定成功";
      var badgeText = data.reused_existing ? "已复用" : "已绑定";
      if (data.stage === "gopay_complete") {
        successText = "支付已完成";
        badgeText = "支付完成";
      } else if (data.reused_existing) {
        successText = "已绑定账号，已直接复用";
      }
      setGopayStep("success", "done", successText);
      if (gopayBadge) { setText(gopayBadge, badgeText); gopayBadge.className = "badge"; }
      setWorkflowButtonState(gopayLinkBtn, "done");
      if (data.stage === "gopay_complete") await saveGPTPlusSuccessRecord(data);
    } else if (data.stage === "otp_all_failed") {
      if (gopayBadge) { setText(gopayBadge, "需真实验证码"); gopayBadge.className = "badge error"; }
      setWorkflowButtonState(gopayLinkBtn, "error");
      showAutoFillNotice("需要真实 OTP", "提示", "自动尝试沙箱测试码均失败。", "请输入从 WhatsApp/SMS 收到的 6 位真实 OTP 验证码后重试。", "error");
    } else if (data.stage === "pin_all_failed" || data.stage === "payment_pin_all_failed") {
      if (gopayBadge) { setText(gopayBadge, data.stage === "payment_pin_all_failed" ? "需支付 PIN" : "PIN 未通过"); gopayBadge.className = "badge error"; }
      setWorkflowButtonState(gopayLinkBtn, "error");
      showAutoFillNotice(
        data.stage === "payment_pin_all_failed" ? "需要支付 PIN" : "需要真实 GoPay PIN",
        "提示",
        data.stage === "payment_pin_all_failed"
          ? "当前账号已复用到支付阶段，但自动尝试支付 PIN 失败。"
          : "需要输入真实 GoPay PIN，当前自动尝试的 PIN 均未通过。",
        data.stage === "payment_pin_all_failed"
          ? "请在 GoPay PIN 输入框里填入真实 6 位 PIN 后重试。"
          : "",
        "error"
      );
    } else if (data.stage === "gopay_payment_failed") {
      if (gopayBadge) { setText(gopayBadge, "支付失败"); gopayBadge.className = "badge error"; }
      setGopayStep("success", "error", "GoPay 支付页返回失败，请重新生成结账链路");
      setWorkflowButtonState(gopayLinkBtn, "error");
      showAutoFillNotice(
        "GoPay 支付失败",
        "已停止",
        "支付页未完成最终扣款，旧结账链路不能继续复用。",
        "请关闭当前旧支付页，重新生成 OpenAI 结账链接后再触发 GoPay。",
        "error"
      );
    } else {
      if (gopayBadge) { setText(gopayBadge, "失败"); gopayBadge.className = "badge error"; }
      setWorkflowButtonState(gopayLinkBtn, "error");
    }

    return data;
  } catch (err) {
    var elapsed = Math.round(performance.now() - startedAt);
    if (gopayLatency) setText(gopayLatency, elapsed + "ms");
    if (gopayLinkStatus) setText(gopayLinkStatus, "网络错误");
    if (gopayOutput) setText(gopayOutput, JSON.stringify({ error: err.message }));
    if (gopayBadge) { setText(gopayBadge, "错误"); gopayBadge.className = "badge error"; }
    setGopayStep("linking", "error", "网络错误: " + err.message);
    setWorkflowButtonState(gopayLinkBtn, "error");
    return null;
  } finally {
    gopayPaymentFlowRunning = false;
    if (gopayLinkBtn) gopayLinkBtn.disabled = false;
    if (gopayMonitorBtn) gopayMonitorBtn.disabled = false;
    setText(gopayLinkBtn, origText);
  }
}

gopayLinkBtn?.addEventListener("click", async function () {
  if (gopayCheckoutWatcherActive) {
    appendCheckoutWatcherEvent("manual-full-link-blocked", "自动监控已接管本次 checkout，已阻止手动 full-link 路径");
    showAutoFillNotice("自动监控运行中", "提示", "当前 checkout 已由自动监控接管。请在支付页点击订阅后等待程序自动填写 Midtrans GoPay 页面。", "手动 GoPay 一键绑定会新建后端 full-link 流程，容易造成回跳状态错乱。", "");
    return;
  }
  await runGopayFullLinkPayment({ triggerSource: "manual" });
});

// =================================
// LuckMail 接码
// =================================
const luckmailProjectCode = document.querySelector("#luckmailProjectCode");
const luckmailEmailType = document.querySelector("#luckmailEmailType");
const luckmailDomain = document.querySelector("#luckmailDomain");
const luckmailSpecifiedEmail = document.querySelector("#luckmailSpecifiedEmail");
const luckmailTimeoutS = document.querySelector("#luckmailTimeoutS");
const luckmailIntervalS = document.querySelector("#luckmailIntervalS");
const luckmailToken = document.querySelector("#luckmailToken");
const luckmailPurchasedEmail = document.querySelector("#luckmailPurchasedEmail");
const luckmailManualCode = document.querySelector("#luckmailManualCode");
const luckmailModeHint = document.querySelector("#luckmailModeHint");
const luckmailTokenTimeoutS = document.querySelector("#luckmailTokenTimeoutS");
const luckmailTokenIntervalS = document.querySelector("#luckmailTokenIntervalS");
const luckmailDeviceLoginGuideBtn = document.querySelector("#luckmailDeviceLoginGuideBtn");
const luckmailLoadPurchaseBtn = document.querySelector("#luckmailLoadPurchaseBtn");
const luckmailLoginEmailFillBtn = document.querySelector("#luckmailLoginEmailFillBtn");
const luckmailManualCodeFillBtn = document.querySelector("#luckmailManualCodeFillBtn");
const luckmailTokenCodeBtn = document.querySelector("#luckmailTokenCodeBtn");
const luckmailTokenMailsBtn = document.querySelector("#luckmailTokenMailsBtn");
const luckmailShareCodeLinkBtn = document.querySelector("#luckmailShareCodeLinkBtn");
const luckmailCreateAndWaitBtn = document.querySelector("#luckmailCreateAndWaitBtn");
const luckmailCopyCodeBtn = document.querySelector("#luckmailCopyCodeBtn");
const luckmailBadge = document.querySelector("#luckmailBadge");
const luckmailApiProfileSelect = document.querySelector("#luckmailApiProfileSelect");
const luckmailApiProfileName = document.querySelector("#luckmailApiProfileName");
const luckmailApiKey = document.querySelector("#luckmailApiKey");
const luckmailApiBaseUrl = document.querySelector("#luckmailApiBaseUrl");
const luckmailApiProjectCode = document.querySelector("#luckmailApiProjectCode");
const luckmailApiEmailType = document.querySelector("#luckmailApiEmailType");
const luckmailApiDomain = document.querySelector("#luckmailApiDomain");
const luckmailApiTimeoutS = document.querySelector("#luckmailApiTimeoutS");
const luckmailApiIntervalS = document.querySelector("#luckmailApiIntervalS");
const luckmailApiTestBtn = document.querySelector("#luckmailApiTestBtn");
const luckmailApiSaveBtn = document.querySelector("#luckmailApiSaveBtn");
const luckmailApiActivateBtn = document.querySelector("#luckmailApiActivateBtn");
const luckmailApiDeleteBtn = document.querySelector("#luckmailApiDeleteBtn");
const luckmailApiStatus = document.querySelector("#luckmailApiStatus");
const luckmailVoicePromptToggle = document.querySelector("#luckmailVoicePromptToggle");
const luckmailVoiceVolume = document.querySelector("#luckmailVoiceVolume");
const luckmailVoiceRate = document.querySelector("#luckmailVoiceRate");
const luckmailVoicePromptText = document.querySelector("#luckmailVoicePromptText");
const luckmailResult = document.querySelector("#luckmailResult");
const luckmailEmailAddress = document.querySelector("#luckmailEmailAddress");
const luckmailVerificationCode = document.querySelector("#luckmailVerificationCode");
const luckmailLatency = document.querySelector("#luckmailLatency");
const luckmailMailMeta = document.querySelector("#luckmailMailMeta");
const luckmailOutput = document.querySelector("#luckmailOutput");
const luckmailMailsPanel = document.querySelector("#luckmailMailsPanel");
const luckmailMailsCount = document.querySelector("#luckmailMailsCount");
const luckmailMailsEmail = document.querySelector("#luckmailMailsEmail");
const luckmailMailsList = document.querySelector("#luckmailMailsList");
let latestLuckMailCode = "";
let luckMailVerificationSinceUnixMS = 0;
let luckMailCodePollingActive = false;
let lastVoicePromptKey = "";
let activeLuckMailVoiceUtterance = null;
let cachedLuckMailVoices = [];
let voicePromptPlaybackQueue = Promise.resolve();
let voicePromptRecentKeys = new Map();
const luckmailVoicePromptStorageKey = "luckmail_voice_prompt_enabled";
const luckmailVoiceVolumeStorageKey = "luckmail_voice_prompt_volume";
const luckmailVoiceRateStorageKey = "luckmail_voice_prompt_rate";
const voicePromptDedupeWindowMS = 6000;

function speechPromptSupported() {
  return "speechSynthesis" in window && "SpeechSynthesisUtterance" in window;
}

function setLuckMailVoicePromptStatus(message) {
  if (luckmailVoicePromptText) setText(luckmailVoicePromptText, message);
}

function clampVoiceNumber(value, fallback, min, max) {
  var number = Number(value);
  if (!Number.isFinite(number)) number = fallback;
  return Math.min(max, Math.max(min, Math.round(number)));
}

function voicePromptVolume() {
  return clampVoiceNumber(luckmailVoiceVolume?.value, 100, 0, 100);
}

function voicePromptRate() {
  return clampVoiceNumber(luckmailVoiceRate?.value, 0, -4, 4);
}

function browserVoiceRate() {
  return Math.min(1.8, Math.max(0.5, 1 + voicePromptRate() / 10));
}

function normalizeVoicePromptMessage(message) {
  var text = String(message || "")
    .replace(/<script[^>]*>[\s\S]*?<\/script>|<style[^>]*>[\s\S]*?<\/style>/gi, " ")
    .replace(/<[^>]+>/g, " ")
    .replace(/[\u0000-\u001f\u007f-\u009f]/g, " ")
    .replace(/[\u200b-\u200d\ufeff\ufe0e\ufe0f]/gi, "")
    .replace(/[\u{1f000}-\u{1faff}\u2600-\u27bf]/gu, " ");
  text = text.replace(/\s+/g, " ").trim();
  var chars = Array.from(text);
  if (chars.length > 160) text = chars.slice(0, 160).join("");
  return text;
}

function voicePromptSettingsSummary() {
  return "音量 " + voicePromptVolume() + "% · 语速 " + voicePromptRate();
}

function pruneVoicePromptRecentKeys(now) {
  now = now || Date.now();
  voicePromptRecentKeys.forEach(function (lastAt, key) {
    if (now - lastAt > Math.max(voicePromptDedupeWindowMS, 12000)) {
      voicePromptRecentKeys.delete(key);
    }
  });
}

function shouldAnnounceVoicePrompt(key, force, windowMS) {
  var promptKey = String(key || "");
  if (!promptKey) return true;
  var now = Date.now();
  pruneVoicePromptRecentKeys(now);
  var dedupeMS = Math.max(800, Number(windowMS || voicePromptDedupeWindowMS));
  var lastAt = Number(voicePromptRecentKeys.get(promptKey) || 0);
  if (!force && lastAt > 0 && now - lastAt < dedupeMS) return false;
  voicePromptRecentKeys.set(promptKey, now);
  return true;
}

function refreshLuckMailVoices() {
  if (!speechPromptSupported()) {
    cachedLuckMailVoices = [];
    return cachedLuckMailVoices;
  }
  try {
    cachedLuckMailVoices = window.speechSynthesis.getVoices() || [];
  } catch (_err) {
    cachedLuckMailVoices = [];
  }
  return cachedLuckMailVoices;
}

function selectLuckMailVoice() {
  var voices = cachedLuckMailVoices.length ? cachedLuckMailVoices : refreshLuckMailVoices();
  return voices.find(function (voice) {
    return /zh[-_]?cn|zh[-_]?hans|mandarin|chinese|xiaoxiao|yunxi|huihui|普通话|中文/i.test((voice.lang || "") + " " + (voice.name || ""));
  }) || voices.find(function (voice) {
    return /^zh/i.test(voice.lang || "");
  }) || null;
}

function luckMailVoiceErrorMessage(reason) {
  if (reason === "not-allowed") return "语音被浏览器拦截，请再点一次开关";
  if (reason === "synthesis-failed") return "语音启动失败，请检查系统音量或浏览器语音包";
  if (reason === "audio-busy") return "语音设备忙，请稍后重试";
  return "语音播放失败：" + (reason || "未知错误");
}

async function speakLuckMailPromptLocally(message) {
  var speechText = normalizeVoicePromptMessage(message);
  if (!speechText) throw new Error("empty voice message");
  var result = await fetchJSONWithTimeout("/api/voice/speak", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify({ message: speechText, volume: voicePromptVolume(), rate: voicePromptRate() }),
  }, 18000);
  if (!result.data?.ok) {
    throw new Error(result.data?.error || result.data?.stage || "本机语音接口失败");
  }
  return result.data;
}

function speakLuckMailPromptInBrowser(key, message, force) {
  if (!speechPromptSupported()) {
    setLuckMailVoicePromptStatus("浏览器不支持语音");
    return false;
  }
  var speechText = normalizeVoicePromptMessage(message);
  if (!speechText) return false;
  var promptKey = String(key || message);
  if (!force && lastVoicePromptKey === promptKey) return false;
  lastVoicePromptKey = promptKey;
  try {
    refreshLuckMailVoices();
    var utterance = new SpeechSynthesisUtterance(speechText);
    var selectedVoice = selectLuckMailVoice();
    if (selectedVoice) {
      utterance.voice = selectedVoice;
      utterance.lang = selectedVoice.lang || "zh-CN";
    } else {
      utterance.lang = "zh-CN";
    }
    utterance.rate = browserVoiceRate();
    utterance.pitch = 1;
    utterance.volume = voicePromptVolume() / 100;
    utterance.onstart = function () {
      setLuckMailVoicePromptStatus(speechText);
    };
    utterance.onend = function () {
      if (activeLuckMailVoiceUtterance === utterance) {
        activeLuckMailVoiceUtterance = null;
      }
    };
    utterance.onerror = function (event) {
      if (activeLuckMailVoiceUtterance === utterance) {
        activeLuckMailVoiceUtterance = null;
      }
      var reason = event?.error || "unknown";
      if (reason === "interrupted" || reason === "canceled") return;
      setLuckMailVoicePromptStatus(luckMailVoiceErrorMessage(reason));
    };
    activeLuckMailVoiceUtterance = utterance;
    window.speechSynthesis.cancel();
    if (typeof window.speechSynthesis.resume === "function") {
      window.speechSynthesis.resume();
    }
    setLuckMailVoicePromptStatus(speechText);
    window.speechSynthesis.speak(utterance);
    window.setTimeout(function () {
      if (luckmailVoicePromptToggle?.checked && window.speechSynthesis.paused && typeof window.speechSynthesis.resume === "function") {
        window.speechSynthesis.resume();
      }
    }, 250);
    return true;
  } catch (error) {
    setLuckMailVoicePromptStatus(luckMailVoiceErrorMessage(error?.message || "未知错误"));
    return false;
  }
}

function speakLuckMailPrompt(key, message, force) {
  var speechText = normalizeVoicePromptMessage(message);
  if (!speechText) return false;
  if (!voicePromptEnabled()) return false;
  var promptKey = String(key || speechText);
  if (!shouldAnnounceVoicePrompt(promptKey, force, voicePromptDedupeWindowMS)) return false;
  lastVoicePromptKey = promptKey;
  setLuckMailVoicePromptStatus("中文语音排队中...");
  voicePromptPlaybackQueue = voicePromptPlaybackQueue.catch(function () {}).then(async function () {
    if (!voicePromptEnabled()) return;
    setLuckMailVoicePromptStatus("本机语音播报中...");
    try {
      await speakLuckMailPromptLocally(speechText);
      setLuckMailVoicePromptStatus(speechText);
    } catch (error) {
      var usedBrowserFallback = speakLuckMailPromptInBrowser(promptKey, speechText, true);
      if (!usedBrowserFallback) {
        setLuckMailVoicePromptStatus("本机语音失败：" + (error?.message || "未知错误"));
      }
    }
  });
  return true;
}

if (speechPromptSupported()) {
  refreshLuckMailVoices();
  if (typeof window.speechSynthesis.addEventListener === "function") {
    window.speechSynthesis.addEventListener("voiceschanged", refreshLuckMailVoices);
  } else {
    window.speechSynthesis.onvoiceschanged = refreshLuckMailVoices;
  }
}

function restoreLuckMailVoicePromptPreference() {
  if (!luckmailVoicePromptToggle) return;
  try {
    luckmailVoicePromptToggle.checked = window.localStorage?.getItem(luckmailVoicePromptStorageKey) === "1";
    if (luckmailVoiceVolume) luckmailVoiceVolume.value = String(clampVoiceNumber(window.localStorage?.getItem(luckmailVoiceVolumeStorageKey), 100, 0, 100));
    if (luckmailVoiceRate) luckmailVoiceRate.value = String(clampVoiceNumber(window.localStorage?.getItem(luckmailVoiceRateStorageKey), 0, -4, 4));
  } catch (_err) {
    luckmailVoicePromptToggle.checked = false;
  }
  setLuckMailVoicePromptStatus(luckmailVoicePromptToggle.checked ? ("语音播报已开启 · " + voicePromptSettingsSummary()) : "未开启");
}

function voicePromptEnabled() {
  return !!(luckmailVoicePromptToggle && luckmailVoicePromptToggle.checked);
}

function announceLuckMailScene(key, message, force) {
  var speechText = normalizeVoicePromptMessage(message);
  if (!speechText) return;
  setLuckMailVoicePromptStatus(speechText);
  if (!voicePromptEnabled()) return;
  speakLuckMailPrompt(key, speechText, force);
}

luckmailVoicePromptToggle?.addEventListener("change", function () {
  try {
    window.localStorage?.setItem(luckmailVoicePromptStorageKey, luckmailVoicePromptToggle.checked ? "1" : "0");
  } catch (_err) {}
  if (luckmailVoicePromptToggle.checked) {
    setLuckMailVoicePromptStatus("正在试播...");
    speakLuckMailPrompt("voice_enabled", "语音播报已开启", true);
  } else {
    if ("speechSynthesis" in window) window.speechSynthesis.cancel();
    activeLuckMailVoiceUtterance = null;
    lastVoicePromptKey = "";
    setLuckMailVoicePromptStatus("未开启");
  }
});

function persistLuckMailVoiceSettings(announce) {
  try {
    window.localStorage?.setItem(luckmailVoiceVolumeStorageKey, String(voicePromptVolume()));
    window.localStorage?.setItem(luckmailVoiceRateStorageKey, String(voicePromptRate()));
  } catch (_err) {}
  if (voicePromptEnabled()) {
    setLuckMailVoicePromptStatus("语音设置已更新 · " + voicePromptSettingsSummary());
    if (announce) speakLuckMailPrompt("voice_settings_updated", "语音设置已更新", true);
  }
}

luckmailVoiceVolume?.addEventListener("input", function () { persistLuckMailVoiceSettings(false); });
luckmailVoiceVolume?.addEventListener("change", function () { persistLuckMailVoiceSettings(true); });
luckmailVoiceRate?.addEventListener("input", function () { persistLuckMailVoiceSettings(false); });
luckmailVoiceRate?.addEventListener("change", function () { persistLuckMailVoiceSettings(true); });

restoreLuckMailVoicePromptPreference();

let luckmailApiConfigState = {
  profiles: [],
  fallback_config: null,
  active_id: "",
  active_source: "config",
};
const luckmailLegacyConfigProfileID = "__config__";
let luckmailApiFormDirty = false;

function profileKeySummary(summary) {
  var key = summary?.api_key || {};
  if (!key.present) return "未配置密钥";
  return "密钥已配置" + (key.suffix ? " · 尾号 " + key.suffix : "") + (key.length ? " · " + key.length + " 位" : "");
}

function allLuckMailAPIProfiles() {
  var profiles = Array.isArray(luckmailApiConfigState.profiles) ? luckmailApiConfigState.profiles.slice() : [];
  if (luckmailApiConfigState.fallback_config) profiles.push(luckmailApiConfigState.fallback_config);
  return profiles;
}

function findLuckMailAPIProfile(id) {
  id = String(id || "");
  return allLuckMailAPIProfiles().find(function (profile) { return profile && profile.id === id; }) || null;
}

function isEditableLuckMailAPIProfileID(id) {
  return !!id && id !== luckmailLegacyConfigProfileID;
}

function selectedLuckMailAPIProfileID() {
  return luckmailApiProfileSelect?.value || "";
}

function luckMailAPIContextPayload() {
  var id = selectedLuckMailAPIProfileID();
  if (!id) return {};
  return { luckmail_api_key_profile_id: id };
}

function fillLuckMailAPIForm(profile) {
  profile = profile || {};
  if (luckmailApiProfileName) luckmailApiProfileName.value = profile.locked ? "" : (profile.name || "");
  if (luckmailApiKey && !(luckmailApiFormDirty && luckmailApiKey.value.trim())) luckmailApiKey.value = "";
  if (luckmailApiBaseUrl) luckmailApiBaseUrl.value = profile.base_url || "";
  if (luckmailApiProjectCode) luckmailApiProjectCode.value = profile.project_code || "";
  if (luckmailApiEmailType) luckmailApiEmailType.value = profile.email_type || "ms_graph";
  if (luckmailApiDomain) luckmailApiDomain.value = profile.domain || "";
  if (luckmailApiTimeoutS) luckmailApiTimeoutS.value = profile.timeout_s || "";
  if (luckmailApiIntervalS) luckmailApiIntervalS.value = profile.interval_s || "";

  if (profile.project_code && luckmailProjectCode) luckmailProjectCode.value = profile.project_code;
  if (profile.email_type && luckmailEmailType) luckmailEmailType.value = profile.email_type;
  if (profile.domain && luckmailDomain) luckmailDomain.value = profile.domain;
  if (profile.timeout_s) {
    if (luckmailTimeoutS) luckmailTimeoutS.value = profile.timeout_s;
    if (luckmailTokenTimeoutS) luckmailTokenTimeoutS.value = profile.timeout_s;
  }
  if (profile.interval_s) {
    if (luckmailIntervalS) luckmailIntervalS.value = profile.interval_s;
    if (luckmailTokenIntervalS) luckmailTokenIntervalS.value = profile.interval_s;
  }
}

function renderLuckMailAPIConfig(data, preferredID) {
  if (!data) return;
  luckmailApiConfigState = {
    profiles: Array.isArray(data.profiles) ? data.profiles : [],
    fallback_config: data.fallback_config || null,
    active_id: data.active_id || "",
    active_source: data.active_source || "config",
  };
  if (!luckmailApiProfileSelect) return;
  var previous = preferredID || luckmailApiProfileSelect.value || "";
  var options = [];
  options.push({ value: "", label: "自动使用活动配置" });
  if (luckmailApiConfigState.fallback_config) {
    options.push({ value: luckmailLegacyConfigProfileID, label: "配置文件 / 环境变量" });
  }
  luckmailApiConfigState.profiles.forEach(function (profile) {
    options.push({
      value: profile.id,
      label: (profile.active ? "默认 · " : "") + (profile.name || profile.id || "未命名配置"),
    });
  });
  luckmailApiProfileSelect.innerHTML = "";
  options.forEach(function (item) {
    var option = document.createElement("option");
    option.value = item.value;
    option.textContent = item.label;
    luckmailApiProfileSelect.appendChild(option);
  });
  var selected = previous;
  if (selected && !options.some(function (item) { return item.value === selected; })) selected = "";
  if (!selected) selected = luckmailApiConfigState.active_id || luckmailLegacyConfigProfileID;
  if (!options.some(function (item) { return item.value === selected; })) selected = "";
  luckmailApiProfileSelect.value = selected;
  updateLuckMailAPIConfigFormFromSelection();
}

function updateLuckMailAPIConfigFormFromSelection() {
  var selectedID = selectedLuckMailAPIProfileID();
  var profile = selectedID ? findLuckMailAPIProfile(selectedID) : (findLuckMailAPIProfile(luckmailApiConfigState.active_id) || luckmailApiConfigState.fallback_config);
  fillLuckMailAPIForm(profile);
  var locked = !!profile?.locked;
  if (luckmailApiDeleteBtn) luckmailApiDeleteBtn.disabled = locked || !isEditableLuckMailAPIProfileID(selectedID);
  if (luckmailApiActivateBtn) luckmailApiActivateBtn.disabled = locked || !isEditableLuckMailAPIProfileID(selectedID) || !!profile?.active;
  if (luckMailCodePollingActive) {
    if (luckmailApiSaveBtn) luckmailApiSaveBtn.disabled = true;
    if (luckmailApiTestBtn) luckmailApiTestBtn.disabled = true;
    if (luckmailApiDeleteBtn) luckmailApiDeleteBtn.disabled = true;
    if (luckmailApiActivateBtn) luckmailApiActivateBtn.disabled = true;
  }
  if (luckmailApiStatus) {
    var activeText = profile?.active ? "当前默认" : (selectedID ? "可设为默认" : "自动选择");
    setText(luckmailApiStatus, luckMailCodePollingActive ? "正在等待验证码，已暂时锁定配置与关闭窗口操作" : [activeText, profile?.name || "配置文件 / 环境变量", profileKeySummary(profile)].filter(Boolean).join(" · "));
  }
}

function setLuckMailCodePollingActive(active) {
  luckMailCodePollingActive = !!active;
  if (closeIncognitoBtn) closeIncognitoBtn.disabled = luckMailCodePollingActive;
  updateLuckMailAPIConfigFormFromSelection();
}

async function loadLuckMailAPIConfig(preferredID) {
  if (!luckmailApiProfileSelect) return;
  try {
    var result = await fetchJSONWithTimeout("/api/luckmail/config", { method: "GET", headers: buildRequestHeaders() }, 12000);
    var data = result.data;
    if (!data.ok) throw new Error(data.error || "读取 LuckMail API 配置失败");
    renderLuckMailAPIConfig(data, preferredID);
  } catch (error) {
    if (luckmailApiStatus) setText(luckmailApiStatus, error.message || "读取 LuckMail API 配置失败");
  }
}

function luckMailAPIConfigFormPayload(setActive) {
  var selectedID = selectedLuckMailAPIProfileID();
  return {
    id: isEditableLuckMailAPIProfileID(selectedID) ? selectedID : "",
    name: (luckmailApiProfileName?.value || "").trim(),
    api_key: (luckmailApiKey?.value || "").trim(),
    base_url: (luckmailApiBaseUrl?.value || "").trim(),
    project_code: (luckmailApiProjectCode?.value || "").trim(),
    email_type: (luckmailApiEmailType?.value || "").trim(),
    domain: (luckmailApiDomain?.value || "").trim(),
    timeout_s: numberInputValue(luckmailApiTimeoutS, 0),
    interval_s: numberInputValue(luckmailApiIntervalS, 0),
    set_active: setActive !== false,
  };
}

async function saveLuckMailAPIConfig(setActive) {
  if (!luckmailApiSaveBtn) return;
  var payload = luckMailAPIConfigFormPayload(true);
  if (!payload.name && !payload.id) payload.name = "主力 API";
  var originalText = setActive ? (luckmailApiActivateBtn?.textContent || "设为默认") : (luckmailApiSaveBtn.textContent || "保存配置");
  var targetButton = setActive ? luckmailApiActivateBtn : luckmailApiSaveBtn;
  if (targetButton) {
    targetButton.disabled = true;
    setText(targetButton, setActive ? "设置中..." : "保存中...");
  }
  if (luckmailApiStatus) setText(luckmailApiStatus, setActive ? "正在设置默认配置..." : "正在保存配置...");
  try {
    var result = await fetchJSONWithTimeout("/api/luckmail/config", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(payload),
    }, 12000);
    var resp = result.response;
    var data = result.data;
    if (!resp.ok || !data.ok) throw new Error(data.error || "保存 LuckMail API 配置失败");
    if (luckmailApiKey) luckmailApiKey.value = "";
    luckmailApiFormDirty = false;
    var nextID = data.saved_id || data.active_id || payload.id || "";
    renderLuckMailAPIConfig(data, nextID);
    if (luckmailApiStatus) setText(luckmailApiStatus, (setActive ? "已设为默认" : "配置已保存并设为当前使用") + " · " + profileKeySummary(findLuckMailAPIProfile(nextID)));
  } catch (error) {
    if (luckmailApiStatus) setText(luckmailApiStatus, error.message || "保存 LuckMail API 配置失败");
  } finally {
    if (targetButton) {
      targetButton.disabled = false;
      setText(targetButton, originalText);
    }
  }
}

async function deleteLuckMailAPIConfig() {
  var selectedID = selectedLuckMailAPIProfileID();
  if (!isEditableLuckMailAPIProfileID(selectedID)) {
    if (luckmailApiStatus) setText(luckmailApiStatus, "请选择一个已保存配置后再删除。");
    return;
  }
  if (luckmailApiDeleteBtn) {
    luckmailApiDeleteBtn.disabled = true;
    setText(luckmailApiDeleteBtn, "删除中...");
  }
  if (luckmailApiStatus) setText(luckmailApiStatus, "正在删除配置...");
  try {
    var result = await fetchJSONWithTimeout("/api/luckmail/config", {
      method: "DELETE",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ id: selectedID }),
    }, 12000);
    var resp = result.response;
    var data = result.data;
    if (!resp.ok || !data.ok) throw new Error(data.error || "删除 LuckMail API 配置失败");
    renderLuckMailAPIConfig(data, data.active_id || luckmailLegacyConfigProfileID);
    if (luckmailApiStatus) setText(luckmailApiStatus, "配置已删除");
  } catch (error) {
    if (luckmailApiStatus) setText(luckmailApiStatus, error.message || "删除 LuckMail API 配置失败");
  } finally {
    if (luckmailApiDeleteBtn) {
      luckmailApiDeleteBtn.disabled = false;
      setText(luckmailApiDeleteBtn, "删除配置");
    }
    updateLuckMailAPIConfigFormFromSelection();
  }
}

async function testLuckMailAPIConfig() {
  if (!luckmailApiTestBtn) return;
  var payload = luckMailAPIConfigFormPayload(false);
  if (!payload.api_key && isEditableLuckMailAPIProfileID(selectedLuckMailAPIProfileID())) {
    payload.id = selectedLuckMailAPIProfileID();
  } else if (!payload.api_key && selectedLuckMailAPIProfileID() === luckmailLegacyConfigProfileID) {
    payload.id = luckmailLegacyConfigProfileID;
  }
  luckmailApiTestBtn.disabled = true;
  setText(luckmailApiTestBtn, "测试中...");
  if (luckmailApiStatus) setText(luckmailApiStatus, "正在测试 LuckMail API Key，最多等待 25 秒...");
  try {
    var result = await fetchJSONWithTimeout("/api/luckmail/config/test", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(payload),
    }, 26000);
    var resp = result.response;
    var data = result.data;
    if (!resp.ok || !data.ok) throw new Error(data.error || "密钥测试失败");
    var account = data.user_info?.username || data.user_info?.email || "账号可用";
    if (luckmailApiStatus) setText(luckmailApiStatus, "测试通过 · " + account + (data.user_info?.balance ? " · 余额 " + data.user_info.balance : ""));
  } catch (error) {
    if (luckmailApiStatus) setText(luckmailApiStatus, error.message || "密钥测试失败");
  } finally {
    luckmailApiTestBtn.disabled = false;
    setText(luckmailApiTestBtn, "测试密钥");
  }
}

function markLuckMailAPIFormDirty() {
  luckmailApiFormDirty = true;
}

luckmailApiProfileSelect?.addEventListener("change", function () {
  luckmailApiFormDirty = false;
  updateLuckMailAPIConfigFormFromSelection();
});
[
  luckmailApiProfileName,
  luckmailApiKey,
  luckmailApiBaseUrl,
  luckmailApiProjectCode,
  luckmailApiEmailType,
  luckmailApiDomain,
  luckmailApiTimeoutS,
  luckmailApiIntervalS,
].forEach(function (node) {
  node?.addEventListener("input", markLuckMailAPIFormDirty);
  node?.addEventListener("change", markLuckMailAPIFormDirty);
});
luckmailApiSaveBtn?.addEventListener("click", function () { saveLuckMailAPIConfig(false); });
luckmailApiActivateBtn?.addEventListener("click", function () { saveLuckMailAPIConfig(true); });
luckmailApiDeleteBtn?.addEventListener("click", deleteLuckMailAPIConfig);
luckmailApiTestBtn?.addEventListener("click", testLuckMailAPIConfig);
loadLuckMailAPIConfig();

luckmailToken?.addEventListener("input", function () {
  if (hasLuckMailToken()) {
    setLuckMailMode("detect", "检测到 Token，读取成功后将自动切换为自动模式");
  } else {
    setLuckMailMode("manual", "未填写 Token，请使用手动邮箱和验证码");
  }
});

luckmailPurchasedEmail?.addEventListener("input", function () {
  if (!hasLuckMailToken() && purchasedEmailValue()) {
    setLuckMailMode("manual", "已填写手动邮箱，验证码需手动输入或提交");
  }
});

function numberInputValue(node, fallback) {
  var value = Number((node?.value || "").trim());
  if (!Number.isFinite(value) || value <= 0) return fallback;
  return Math.round(value);
}

function renderLuckMailData(data, elapsed) {
  if (data?.email_address && hasLuckMailToken()) {
    applyLuckMailPurchase({ email_address: data.email_address, luckmail_token: (luckmailToken?.value || "").trim() });
  }
  latestLuckMailCode = data.verification_code || "";
  setText(luckmailEmailAddress, data.email_address || "-");
  setText(luckmailVerificationCode, latestLuckMailCode || "-");
  setText(luckmailLatency, (data.elapsed_ms || elapsed) + "ms");
  setText(luckmailMailMeta, [data.mail_from || data.project, data.mail_subject || (data.has_new_mail ? "已发现新邮件" : "未发现新邮件")].filter(Boolean).join(" · ") || (data.error || data.status || "-"));
  renderLuckMailOutput(data);
  if (luckmailCopyCodeBtn) luckmailCopyCodeBtn.disabled = !latestLuckMailCode;
  if (luckmailBadge) {
    setText(luckmailBadge, data.ok ? "已收到" : "未收到");
    luckmailBadge.className = data.ok ? "badge" : "badge error";
  }
  if (latestLuckMailCode) {
    announceLuckMailScene("token_code_received", "验证码已收到", true);
  } else if (data.ok) {
    announceLuckMailScene("token_mail_checked", data.has_new_mail ? "已发现新邮件，但未提取到验证码" : "已查询邮箱，暂未发现验证码");
  } else {
    announceLuckMailScene("token_code_failed", data.error || "验证码查询失败");
  }
}

async function runLuckMailCodeFill(code) {
  var cleanCode = String(code || "").trim();
  if (!cleanCode) return null;
  if (luckmailBadge) { setText(luckmailBadge, "填验证码"); luckmailBadge.className = "badge neutral"; }
  setText(luckmailMailMeta, "已收到验证码，正在无痕窗口验证码页面自动填入并提交...");
  announceLuckMailScene("code_fill_started", "正在填写验证码并提交");
  var response = await fetch("/api/login/code-fill", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify({ code: cleanCode, timeout_s: 60 }),
  });
  var data = await response.json();
  renderLuckMailOutput(data);
  if (luckmailBadge) {
    setText(luckmailBadge, data.login_completed ? "登录完成" : (data.profile_filled ? "资料已填" : (data.code_rejected ? "验证码错误" : (data.code_filled ? "已填验证码" : "填码失败"))));
    luckmailBadge.className = data.ok ? "badge" : "badge error";
  }
  var profileMessage = data.profile_filled ? ("已自动填写资料页：" + [data.profile_name, data.profile_age ? (data.profile_age + "岁") : ""].filter(Boolean).join(" · ")) : "";
  setText(luckmailMailMeta, data.login_completed ? "验证码已自动填入并提交，登录流程已完成。" : (profileMessage || (data.code_rejected ? "页面提示验证码错误，准备等待新的验证码。" : (data.code_filled ? "验证码已自动填入并提交，等待页面完成跳转。" : (data.error || "验证码自动填入失败")))));
  if (data.login_completed) {
    clearLuckMailResolvedEmailAfterCompletion();
    announceLuckMailScene("login_completed", "验证码已提交，登录流程已完成", true);
  } else if (data.profile_filled) {
    announceLuckMailScene("login_profile_filled", profileMessage || "资料页已自动填写并提交", true);
  } else if (data.code_rejected) {
    announceLuckMailScene("code_rejected", "页面提示验证码错误，准备等待新的验证码");
  } else if (data.code_filled) {
    announceLuckMailScene("code_filled", "验证码已填入并提交，等待页面跳转");
  } else {
    announceLuckMailScene("code_fill_failed", data.error || "验证码自动填入失败");
  }
  return data;
}

async function waitTokenCodeAndFillLoginCode(options) {
  if (!hasLuckMailToken()) {
    switchLuckMailManualMode("验证码页面已出现，但未读取到有效 Token，请手动填写收到的验证码后点击手动填验证码。");
    return { ok: false, stage: "manual_code_required", manual_mode: true, error: "luckmail_token is required" };
  }
  var maxAttempts = Math.max(1, Number(options?.maxAttempts || 1));
  var lastResult = null;
  for (var attempt = 1; attempt <= maxAttempts; attempt += 1) {
    setText(luckmailMailMeta, attempt > 1 ? ("验证码页仍在等待，正在第 " + attempt + " 次获取新的 LuckMail 验证码...") : "验证码页已出现，正在通过 LuckMail 已购邮箱 Token 等待验证码...");
    announceLuckMailScene(attempt > 1 ? "verification_code_retry_waiting" : "verification_page_ready", attempt > 1 ? "正在等待新的邮箱验证码" : "已进入验证码页面，正在等待邮箱验证码");
    if (luckmailBadge) { setText(luckmailBadge, attempt > 1 ? ("重试 " + attempt + "/" + maxAttempts) : "等验证码"); luckmailBadge.className = "badge neutral"; }
    var data = await fetchLuckMailTokenCodeWithRecovery({ retryableAttempts: Math.max(3, Number(options?.retryableAttempts || 3)) });
    if (data.email_address) applyLuckMailPurchase({ email_address: data.email_address, luckmail_token: (luckmailToken?.value || "").trim() });
    latestLuckMailCode = data.verification_code || "";
    setText(luckmailEmailAddress, data.email_address || purchasedEmailValue() || "-");
    setText(luckmailVerificationCode, latestLuckMailCode || "-");
    setText(luckmailLatency, (data.elapsed_ms || "-") + "ms");
    renderLuckMailOutput(data);
    if (!latestLuckMailCode) {
      if (isLuckMailTokenCodeRetryable(data)) {
        if (luckmailBadge) { setText(luckmailBadge, "等待中断"); luckmailBadge.className = "badge error"; }
        setText(luckmailMailMeta, "LuckMail 验证码等待被连续中断，请保持页面不刷新后再次点击 LuckMail 接码。");
        announceLuckMailScene("token_code_retryable_failed", "验证码等待连续中断，请再次点击 LuckMail 接码");
        return data;
      }
      if (luckmailBadge) { setText(luckmailBadge, "手动模式"); luckmailBadge.className = "badge neutral"; }
      switchLuckMailManualMode(tokenFallbackMessage(data));
      renderLuckMailOutput(Object.assign({}, data, { manual_mode: true }));
      return data;
    }
    announceLuckMailScene("verification_code_received", attempt > 1 ? "新的验证码已收到，正在自动填入" : "验证码已收到，正在自动填入", true);
    if (luckmailCopyCodeBtn) luckmailCopyCodeBtn.disabled = false;
    lastResult = await runLuckMailCodeFill(latestLuckMailCode);
    if (!lastResult?.code_rejected) return lastResult;
    if (attempt >= maxAttempts) return lastResult;
    luckMailVerificationSinceUnixMS = Date.now() - 1000;
    latestLuckMailCode = "";
    setText(luckmailVerificationCode, "-");
    setText(luckmailMailMeta, "验证码被页面判定错误，已切换为只等待新的邮件验证码后重试。");
    await new Promise(function (resolve) { setTimeout(resolve, 1500); });
  }
  return lastResult;
}

function luckMailTokenPayload() {
  var payload = Object.assign({
    luckmail_token: (luckmailToken?.value || "").trim(),
    timeout_s: numberInputValue(luckmailTokenTimeoutS, 300),
    interval_s: numberInputValue(luckmailTokenIntervalS, 3),
  }, luckMailAPIContextPayload());
  if (luckMailVerificationSinceUnixMS > 0) payload.since_unix_ms = luckMailVerificationSinceUnixMS;
  return payload;
}

function luckMailShareCodeLink() {
  var token = (luckmailToken?.value || "").trim();
  if (!token) return "";
  return location.origin + "/mail-code?token=" + encodeURIComponent(token);
}

async function copyLuckMailShareCodeLink() {
  var link = luckMailShareCodeLink();
  if (!link) {
    switchLuckMailManualMode("未填写 LuckMail Token，无法生成验证码查看链接。");
    renderLuckMailOutput({ ok: false, stage: "luckmail_share_link_failed", error: "luckmail_token is required" });
    return;
  }
  await navigator.clipboard.writeText(link);
  setText(luckmailMailMeta, "已复制验证码查看链接，可发送给邮箱使用者打开查看验证码。");
  if (luckmailBadge) { setText(luckmailBadge, "链接已复制"); luckmailBadge.className = "badge"; }
  renderLuckMailOutput({ ok: true, stage: "luckmail_share_link_copied", code_view_url: link });
}

function purchasedEmailValue() {
  return (luckmailPurchasedEmail?.value || "").trim();
}

async function saveGPTPlusSuccessRecord(paymentData) {
  var email = purchasedEmailValue() || (fields.customerEmail?.value || "").trim();
  var token = (luckmailToken?.value || "").trim();
  var checkoutURL = latestOpenedCheckoutURL || latestCheckoutURL || "";
  if (!email || !token) {
    appendCheckoutWatcherEvent("gptpls-record-skipped", "GPT Plus 成功记录未保存：缺少" + (!email && !token ? "邮箱和 LuckMail Token" : (!email ? "邮箱" : "LuckMail Token")));
    return null;
  }
  var recordKey = [email.toLowerCase(), token, checkoutURL].join("|");
  if (gptPlusSuccessRecordKeys.has(recordKey)) {
    appendCheckoutWatcherEvent("gptpls-record-duplicate", "GPT Plus 成功记录已保存过，本次跳过重复写入");
    return null;
  }
  gptPlusSuccessRecordKeys.add(recordKey);
  try {
    var response = await fetch("/api/gptpls/record", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({
        email_address: email,
        luckmail_token: token,
        payment_stage: paymentData?.stage || "gopay_complete",
        payment_method: activePaymentLabel || "GoPay",
        checkout_url: checkoutURL,
      }),
    });
    var data = await response.json();
    if (!data.ok) gptPlusSuccessRecordKeys.delete(recordKey);
    appendCheckoutWatcherEvent("gptpls-record", data.ok ? ("已保存 GPT Plus 成功记录：" + (data.file_path || "gptpls") + (data.code_view_url ? " · 验证码查看链接已生成" : "")) : (data.error || "GPT Plus 成功记录保存失败"));
    return data;
  } catch (error) {
    gptPlusSuccessRecordKeys.delete(recordKey);
    appendCheckoutWatcherEvent("gptpls-record-error", error.message || "GPT Plus 成功记录保存失败");
    return null;
  }
}

function hasLuckMailToken() {
  return isLikelyLuckMailMailboxToken(luckMailTokenValue());
}

function luckMailTokenValue() {
  return (luckmailToken?.value || "").trim();
}

function hasLuckMailTokenInput() {
  return !!luckMailTokenValue();
}

function isLikelyLuckMailMailboxToken(value) {
  return /^tok_[a-z0-9_-]{6,}$/i.test(String(value || "").trim());
}

function invalidLuckMailTokenInputMessage() {
  return "LuckMail Token 输入框中的值不是已购邮箱 Token（应以 tok_ 开头）。已改用 API Key 配置读取已购邮箱列表。";
}

function setLuckMailMode(mode, message) {
  if (!luckmailModeHint) return;
  var label = mode === "auto" ? "自动模式" : (mode === "manual" ? "手动模式" : "自动检测");
  luckmailModeHint.textContent = "当前模式：" + label + (message ? " · " + message : "");
  luckmailModeHint.dataset.mode = mode || "detect";
}

function switchLuckMailManualMode(message) {
  setLuckMailMode("manual", message || "请手动填写邮箱地址和验证码");
  if (luckmailBadge) { setText(luckmailBadge, "手动模式"); luckmailBadge.className = "badge neutral"; }
  setText(luckmailMailMeta, message || "未读取到有效 LuckMail Token，请手动填写邮箱地址和验证码。");
  announceLuckMailScene("manual_mode_enabled", message || "已切换为手动模式，请手动填写邮箱和验证码");
}

function switchLuckMailAutoMode(message) {
  setLuckMailMode("auto", message || "将自动读取邮箱并等待验证码");
}

function tokenFallbackMessage(data) {
  var raw = String(data?.error || data?.message || "").trim();
  if (hasLuckMailTokenInput() && !hasLuckMailToken()) return "LuckMail Token 格式不正确，已切换为手动模式；API Key 请保存到 LuckMail API 配置区域。";
  if (!hasLuckMailToken()) return "未填写 LuckMail Token，已切换为手动模式。";
  if (/context canceled|cancelled|aborted|中断|取消/i.test(raw)) {
    return "验证码等待被中断。系统会自动恢复等待；等待期间不要刷新页面、关闭无痕窗口或修改 LuckMail 配置。";
  }
  if (/expired|expire|过期|invalid|无效|not found|不存在|alive|disabled|不可用/i.test(raw)) {
    return "LuckMail Token 无效、过期或邮箱不可用，已切换为手动模式。";
  }
  return raw ? (raw + "，已切换为手动模式。") : "未读取到有效 LuckMail Token，已切换为手动模式。";
}

function isLuckMailTokenCodeRetryable(data) {
  var raw = String(data?.error || data?.message || data?.stage || "").trim();
  return !!(data?.retryable || data?.interrupted || /context canceled|cancelled|canceled|aborted|operation was aborted|client disconnected|connection reset|broken pipe|中断|取消/i.test(raw));
}

function luckMailTokenCodeRetryDelayMS(attempt) {
  return Math.min(3500, 900 + attempt * 650);
}

async function fetchLuckMailTokenCodeOnce() {
  var response;
  setLuckMailCodePollingActive(true);
  try {
    response = await fetch("/api/luckmail/token-code", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(luckMailTokenPayload()),
    });
  } finally {
    setLuckMailCodePollingActive(false);
  }
  return await response.json();
}

async function fetchLuckMailTokenCodeWithRecovery(options) {
  options = options || {};
  var maxRetryableAttempts = Math.max(1, Number(options.retryableAttempts || 3));
  var lastData = null;
  for (var retryAttempt = 1; retryAttempt <= maxRetryableAttempts; retryAttempt += 1) {
    try {
      lastData = await fetchLuckMailTokenCodeOnce();
    } catch (error) {
      lastData = {
        ok: false,
        stage: "luckmail_token_code_request_interrupted",
        retryable: true,
        interrupted: true,
        error: error.message || "LuckMail 验证码请求被中断",
      };
    }
    if (!isLuckMailTokenCodeRetryable(lastData) || retryAttempt >= maxRetryableAttempts) {
      return lastData;
    }
    var delayMS = luckMailTokenCodeRetryDelayMS(retryAttempt);
    if (luckmailBadge) {
      setText(luckmailBadge, "恢复等待 " + retryAttempt + "/" + maxRetryableAttempts);
      luckmailBadge.className = "badge neutral";
    }
    setText(luckmailMailMeta, "LuckMail 验证码等待连接被中断，正在自动恢复轮询；请不要刷新页面或关闭无痕窗口。");
    announceLuckMailScene("token_code_retryable_interrupted", "验证码等待被中断，正在自动恢复等待");
    renderLuckMailOutput(Object.assign({}, lastData, { auto_retry_attempt: retryAttempt, auto_retry_delay_ms: delayMS }));
    await new Promise(function (resolve) { setTimeout(resolve, delayMS); });
  }
  return lastData;
}

function formatLuckMailDate(value) {
  if (!value) return "时间未知";
  var date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN", { hour12: false });
}

function maskLuckMailDisplayValue(value) {
  var text = String(value || "");
  if (text.length <= 8) return "***";
  return text.slice(0, 4) + "***" + text.slice(-4);
}

function sanitizeLuckMailDisplayPayload(value) {
  if (Array.isArray(value)) {
    return value.map(sanitizeLuckMailDisplayPayload);
  }
  if (!value || typeof value !== "object") {
    return value;
  }
  var result = {};
  Object.keys(value).forEach(function (key) {
    var lowered = key.toLowerCase();
    if (lowered === "luckmail_token" || lowered === "token" || lowered.includes("api_key")) {
      result[key] = maskLuckMailDisplayValue(value[key]);
      return;
    }
    result[key] = sanitizeLuckMailDisplayPayload(value[key]);
  });
  return result;
}

const luckMailStageVoiceMessages = {
  login_click_started: "正在监控登录入口与邮箱登录窗口。",
  login_click_error: "登录按钮自动点击失败，请查看无痕窗口状态。",
  luckmail_purchases_loading: "正在读取 LuckMail 已购邮箱。",
  luckmail_email_preloaded: "已按当前 Token 读取并填入已购邮箱。",
  luckmail_email_preloaded_from_purchases: "已从已购邮箱列表读取并填入邮箱。",
  luckmail_email_preload_failed: "已购邮箱读取失败，请手动检查 Token 或邮箱状态。",
  device_login_guide_started: "设备登录引导已启动。",
  device_login_guide_failed: "设备登录引导失败，请查看页面状态。",
  login_email_fill_started: "正在填写登录邮箱，并等待验证码页面出现。",
  manual_email_required: "需要手动填写邮箱或检查 LuckMail Token。",
  luckmail_token_waiting: "正在等待已购邮箱验证码。",
  luckmail_token_code_received: "已购邮箱验证码已收到，正在自动填入。",
  luckmail_token_code_interrupted: "验证码等待被中断，正在尝试恢复。",
  luckmail_token_invalid_format: "LuckMail Token 格式不正确，请重新粘贴。",
  luckmail_token_mails_loading: "正在查询已购邮箱邮件列表。",
  luckmail_waiting: "正在创建临时接码订单并等待验证码。",
  manual_code_required: "需要手动输入验证码。",
  manual_code_fill_failed: "手动验证码填入失败，请查看页面提示。",
  luckmail_manual_mode: "已进入手动模式，请手动处理邮箱或验证码。",
  luckmail_share_link_copied: "验证码查看链接已复制。",
  luckmail_share_link_failed: "验证码查看链接生成失败。",
};

function announceLuckMailDataStage(data) {
  var stage = String(data?.stage || data?.status || "").trim();
  if (!stage) return;
  var message = luckMailStageVoiceMessages[stage] || "";
  if (!message) {
    if (data?.ok === false && data?.error) message = String(data.error);
    if (data?.verification_code || data?.code) message = "验证码已收到，正在处理。";
  }
  if (!message) return;
  announceLuckMailScene("luckmail_stage_" + stage, message, false);
}

function renderLuckMailOutput(data) {
  setText(luckmailOutput, JSON.stringify(sanitizeLuckMailDisplayPayload(data), null, 2));
  announceLuckMailDataStage(data);
}

function renderLuckMailMails(data) {
  var mails = Array.isArray(data?.mails) ? data.mails : [];
  if (luckmailMailsPanel) luckmailMailsPanel.hidden = false;
  setText(luckmailMailsCount, mails.length + " 封");
  setText(luckmailMailsEmail, data?.email_address || purchasedEmailValue() || "-");
  if (!luckmailMailsList) return;
  luckmailMailsList.innerHTML = "";
  if (!mails.length) {
    var empty = document.createElement("div");
    empty.className = "luckmail-mails-empty";
    empty.textContent = data?.ok ? "LuckMail 当前没有返回邮件。" : (data?.error || "邮件列表查询失败");
    luckmailMailsList.appendChild(empty);
    return;
  }
  mails.forEach(function (mail, index) {
    var item = document.createElement("article");
    item.className = "luckmail-mail-card";

    var head = document.createElement("div");
    head.className = "luckmail-mail-card__head";

    var number = document.createElement("span");
    number.className = "luckmail-mail-card__number";
    number.textContent = "#" + (index + 1);

    var time = document.createElement("span");
    time.className = "luckmail-mail-card__time";
    time.textContent = formatLuckMailDate(mail.received_at);

    head.appendChild(number);
    head.appendChild(time);

    var subject = document.createElement("strong");
    subject.className = "luckmail-mail-card__subject";
    subject.textContent = mail.subject || "无主题";

    var from = document.createElement("p");
    from.className = "luckmail-mail-card__from";
    from.textContent = mail.from ? ("发件人：" + mail.from) : "发件人未知";

    var message = document.createElement("p");
    message.className = "luckmail-mail-card__id";
    message.textContent = mail.message_id ? ("Message ID: " + mail.message_id) : "无 Message ID";

    item.appendChild(head);
    item.appendChild(subject);
    item.appendChild(from);
    item.appendChild(message);
    luckmailMailsList.appendChild(item);
  });
}

function applyLuckMailPurchase(item) {
  if (!item) return false;
  var email = (item.email_address || "").trim();
  var token = (item.luckmail_token || "").trim();
  if (luckmailPurchasedEmail && email) luckmailPurchasedEmail.value = email;
  if (luckmailToken && token) luckmailToken.value = token;
  if (fields.customerEmail && email) fields.customerEmail.value = email;
  setText(luckmailEmailAddress, email || "-");
  return !!email;
}

function clearLuckMailResolvedEmailAfterCompletion() {
  if (!hasLuckMailToken()) return;
  if (luckmailPurchasedEmail) luckmailPurchasedEmail.value = "";
  if (fields.customerEmail) fields.customerEmail.value = "";
  setText(luckmailEmailAddress, "-");
  luckMailVerificationSinceUnixMS = 0;
}

async function resolveLuckMailEmailByToken() {
  if (!hasLuckMailToken()) {
    return { ok: false, manual_mode: true, error: hasLuckMailTokenInput() ? invalidLuckMailTokenInputMessage() : "luckmail_token is required" };
  }
  setText(luckmailMailMeta, "正在按当前 Token 精确读取对应邮箱...");
  announceLuckMailScene("purchase_loading", "正在按 Token 读取对应邮箱");
  var startedAt = performance.now();
  var tokenValue = luckMailTokenValue();
  var tokenResp = await fetch("/api/luckmail/token-mails", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify(luckMailTokenPayload()),
  });
  var tokenData = await tokenResp.json();
  var tokenElapsed = Math.round(performance.now() - startedAt);
  var tokenSelected = tokenData.email_address ? { email_address: tokenData.email_address, luckmail_token: tokenValue } : null;
  var tokenApplied = tokenResp.ok && tokenData.ok && applyLuckMailPurchase(tokenSelected);
  setText(luckmailLatency, (tokenData.elapsed_ms || tokenElapsed) + "ms");
  renderLuckMailOutput(tokenApplied ? Object.assign({}, tokenData, { stage: "luckmail_email_preloaded" }) : Object.assign({}, tokenData, { stage: "luckmail_email_preload_failed", manual_mode: true }));
  if (tokenApplied) {
    if (luckmailBadge) { setText(luckmailBadge, "自动模式"); luckmailBadge.className = "badge"; }
    switchLuckMailAutoMode("已按当前 Token 精确读取并填入对应邮箱");
    setText(luckmailMailMeta, "已按当前 Token 读取并填入：" + tokenSelected.email_address);
    announceLuckMailScene("purchase_loaded", "已按当前 Token 读取并填入已购邮箱");
    return { ok: true, email: tokenSelected.email_address, data: tokenData, token_mode: true };
  }
  var fallback = tokenFallbackMessage(tokenData);
  switchLuckMailManualMode(fallback);
  return { ok: false, manual_mode: true, error: fallback, data: tokenData, token_mode: true };
}

async function resolveLuckMailEmailByPurchaseList(options) {
  options = options || {};
  var startedAt = performance.now();
  if (options.reason) {
    setText(luckmailMailMeta, options.reason);
  } else {
    setText(luckmailMailMeta, "正在从 LuckMail 已购邮箱列表读取可用邮箱...");
  }
  var resp = await fetch("/api/luckmail/purchases", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify(Object.assign({ page: 1, page_size: 20 }, luckMailAPIContextPayload())),
  });
  var data = await resp.json();
  var elapsed = Math.round(performance.now() - startedAt);
  var selected = data.selected || (data.list || []).find(function (item) { return item.email_address && item.luckmail_token && item.user_disabled === 0; }) || null;
  var applied = resp.ok && data.ok && applyLuckMailPurchase(selected);
  setText(luckmailLatency, (data.elapsed_ms || elapsed) + "ms");
  setText(luckmailMailMeta, applied ? ("已读取并填入：" + selected.email_address) : (data.error || "未找到可用已购邮箱"));
  renderLuckMailOutput(applied ? Object.assign({}, data, { stage: "luckmail_email_preloaded_from_purchases" }) : Object.assign({}, data, { stage: "luckmail_email_preload_failed", manual_mode: true }));
  if (luckmailBadge) {
    setText(luckmailBadge, applied ? "已读取" : "未找到");
    luckmailBadge.className = applied ? "badge" : "badge error";
  }
  if (applied) {
    switchLuckMailAutoMode("已通过 API Key 配置读取并填入已购邮箱");
    announceLuckMailScene("purchase_loaded", "已读取并填入已购邮箱");
    return { ok: true, email: selected.email_address, data: data, purchase_list_mode: true };
  }
  var fallback = data.error || "未找到可用已购邮箱，请手动填写邮箱地址和验证码。";
  switchLuckMailManualMode(fallback);
  announceLuckMailScene("purchase_load_failed", fallback);
  return { ok: false, manual_mode: true, error: fallback, data: data, purchase_list_mode: true };
}

async function ensureLuckMailEmailForGuide() {
  if (hasLuckMailToken()) return await resolveLuckMailEmailByToken();
  var existingEmail = purchasedEmailValue();
  if (existingEmail) {
    if (fields.customerEmail) fields.customerEmail.value = existingEmail;
    setText(luckmailEmailAddress, existingEmail);
    setLuckMailMode("manual", "已使用手动邮箱，验证码需要手动填写");
    return { ok: true, email: existingEmail, reused: true };
  }
  if (hasLuckMailTokenInput() && !hasLuckMailToken()) {
    return await resolveLuckMailEmailByPurchaseList({ reason: invalidLuckMailTokenInputMessage() });
  }
  return await resolveLuckMailEmailByPurchaseList();
}

async function runLuckMailDeviceLoginGuide() {
  if (!luckmailDeviceLoginGuideBtn) return;
  var originalText = luckmailDeviceLoginGuideBtn.textContent;
  luckmailDeviceLoginGuideBtn.disabled = true;
  setWorkflowButtonState(luckmailDeviceLoginGuideBtn, "running");
  if (luckmailLoadPurchaseBtn) luckmailLoadPurchaseBtn.disabled = true;
  if (luckmailLoginEmailFillBtn) luckmailLoginEmailFillBtn.disabled = true;
  if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = true;
  if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = true;
  if (luckmailResult) luckmailResult.hidden = false;
  if (luckmailBadge) { setText(luckmailBadge, "引导中"); luckmailBadge.className = "badge neutral"; }
  setText(luckmailVerificationCode, "-");
  setText(luckmailMailMeta, "正在启动设备登录引导：读取 Token 与已购邮箱、打开无痕窗口、识别登录入口并等待验证码...");
  announceLuckMailScene("device_login_guide_started", "设备登录引导已启动");
  renderLuckMailOutput({ stage: "device_login_guide_started" });
  setLuckMailMode(hasLuckMailToken() ? "auto" : "manual", hasLuckMailToken() ? "将先读取邮箱再打开登录窗口" : "未填写 Token，将使用手动邮箱和验证码");
  setText(luckmailDeviceLoginGuideBtn, "读取邮箱...");
  try {
    var emailReady = await ensureLuckMailEmailForGuide();
    if (!emailReady.ok) throw new Error(emailReady.error || "未读取到可用邮箱，请手动填写邮箱地址和验证码");
    setText(luckmailDeviceLoginGuideBtn, "启动窗口...");
    setText(luckmailMailMeta, "已准备邮箱，正在启动无痕窗口并识别登录入口...");
    var opened = await ensureIncognitoWindow();
    if (!opened) throw new Error("无法启动 Chrome 无痕窗口");
    setText(luckmailDeviceLoginGuideBtn, "识别登录...");
    setText(luckmailMailMeta, "无痕窗口已打开，正在确认登录/注册邮箱窗口...");
    var loginData = await runLoginClickAfterIncognitoOpen({ returnData: true });
    var loginReady = !!(loginData && loginData.ok && (loginData.email_input_ready || loginData.email_mode_switched || loginData.login_surface_ready));
    if (!loginReady) {
      var detail = Object.assign({}, loginData || {}, { ok: false, stage: "device_login_guide_login_click_watch_continues", device_login_guide_watch_continues: true });
      renderLuckMailOutput(detail);
      setText(luckmailMailMeta, "登录弹窗未立即出现，仍将继续监控；只要登录或注册邮箱窗口出现就会自动填写邮箱。");
      announceLuckMailScene("device_login_guide_login_watch_continues", "登录弹窗未立即出现，系统继续监控邮箱输入窗口");
    }
    if (!purchasedEmailValue()) {
      switchLuckMailManualMode("未读取到可用邮箱，请手动填写邮箱地址和验证码。");
      throw new Error("未读取到可用邮箱，请手动填写邮箱地址和验证码");
    }
    if (fields.customerEmail) fields.customerEmail.value = purchasedEmailValue();
    setText(luckmailDeviceLoginGuideBtn, "填邮箱...");
    setText(luckmailMailMeta, "已准备邮箱，正在填入登录页并等待验证码...");
    var guideResult = await runLuckMailLoginEmailFill({ codeRetry: { maxAttempts: 3 }, controlButton: luckmailDeviceLoginGuideBtn });
    if (guideResult?.login_completed) {
      if (luckmailBadge) { setText(luckmailBadge, "登录完成"); luckmailBadge.className = "badge"; }
      setWorkflowButtonState(luckmailDeviceLoginGuideBtn, "done");
      announceLuckMailScene("device_login_guide_finished", "设备登录引导已完成，登录成功", true);
    } else if (isLuckMailTokenCodeRetryable(guideResult)) {
      if (luckmailBadge) { setText(luckmailBadge, "等待中断"); luckmailBadge.className = "badge error"; }
      setWorkflowButtonState(luckmailDeviceLoginGuideBtn, "error");
      setText(luckmailMailMeta, "设备登录引导已提交邮箱，但验证码等待连接被连续中断；请保持页面不刷新后重新点击 LuckMail 接码。");
      announceLuckMailScene("device_login_guide_code_wait_interrupted", "验证码等待连续中断，请重新点击 LuckMail 接码");
    } else if (guideResult?.code_rejected) {
      if (luckmailBadge) { setText(luckmailBadge, "验证码错误"); luckmailBadge.className = "badge error"; }
      setWorkflowButtonState(luckmailDeviceLoginGuideBtn, "error");
      announceLuckMailScene("device_login_guide_finished_with_code_error", "设备登录引导已重试，但验证码仍被页面判定错误");
    } else {
      if (luckmailBadge && latestLuckMailCode) { setText(luckmailBadge, "已填验证码"); luckmailBadge.className = "badge"; }
      setWorkflowButtonState(luckmailDeviceLoginGuideBtn, "done");
      announceLuckMailScene("device_login_guide_finished", latestLuckMailCode ? "设备登录引导已完成，验证码已填入" : "设备登录引导已执行，请查看页面状态", true);
    }
  } catch (error) {
    if (luckmailBadge) { setText(luckmailBadge, "引导失败"); luckmailBadge.className = "badge error"; }
    setWorkflowButtonState(luckmailDeviceLoginGuideBtn, "error");
    setText(luckmailMailMeta, error.message || "设备登录引导失败");
    announceLuckMailScene("device_login_guide_failed", error.message || "设备登录引导失败");
    renderLuckMailOutput(error.detail || { ok: false, stage: "device_login_guide_failed", error: error.message });
  } finally {
    luckmailDeviceLoginGuideBtn.disabled = false;
    if (luckmailLoadPurchaseBtn) luckmailLoadPurchaseBtn.disabled = false;
    if (luckmailLoginEmailFillBtn) luckmailLoginEmailFillBtn.disabled = false;
    if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = false;
    if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = false;
    setText(luckmailDeviceLoginGuideBtn, originalText || "设备登录引导");
  }
}

async function runLuckMailLoadPurchase() {
  if (!luckmailLoadPurchaseBtn) return;
  var originalText = luckmailLoadPurchaseBtn.textContent;
  var startedAt = performance.now();
  luckmailLoadPurchaseBtn.disabled = true;
  if (luckmailLoginEmailFillBtn) luckmailLoginEmailFillBtn.disabled = true;
  if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = true;
  if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = true;
  setText(luckmailLoadPurchaseBtn, "读取中...");
  if (luckmailBadge) { setText(luckmailBadge, "读取中"); luckmailBadge.className = "badge neutral"; }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailMailMeta, (hasLuckMailToken() ? "正在按当前 Token 精确读取绑定邮箱..." : "正在从 LuckMail 已购邮箱列表读取可用邮箱..."));
  announceLuckMailScene("purchase_loading", "正在读取已购邮箱");
  renderLuckMailOutput({ stage: "luckmail_purchases_loading" });

  try {
    if (hasLuckMailToken()) {
      await resolveLuckMailEmailByToken();
      return;
    }
    var reason = hasLuckMailTokenInput() ? invalidLuckMailTokenInputMessage() : "";
    await resolveLuckMailEmailByPurchaseList(reason ? { reason: reason } : {});
  } catch (error) {
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, elapsed + "ms");
    setText(luckmailMailMeta, error.message || "LuckMail 已购邮箱读取失败");
    announceLuckMailScene("purchase_error", error.message || "LuckMail 已购邮箱读取失败");
    renderLuckMailOutput({ ok: false, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
  } finally {
    luckmailLoadPurchaseBtn.disabled = false;
    if (luckmailLoginEmailFillBtn) luckmailLoginEmailFillBtn.disabled = false;
    if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = false;
    if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = false;
    setText(luckmailLoadPurchaseBtn, originalText || "读取已购邮箱");
  }
}

async function runLuckMailLoginEmailFill(options) {
  options = options || {};
  var controlButton = options.controlButton || luckmailLoginEmailFillBtn || null;
  var emailReady = hasLuckMailToken() ? await ensureLuckMailEmailForGuide() : { ok: !!purchasedEmailValue(), email: purchasedEmailValue() };
  var email = String(emailReady?.email || purchasedEmailValue() || "").trim();
  if (fields.customerEmail) fields.customerEmail.value = email;
  if (!emailReady?.ok || !email) {
    if (luckmailResult) luckmailResult.hidden = false;
    var fillMessage = hasLuckMailToken() ? (emailReady?.error || "未按 Token 读取到有效邮箱，请检查 Token 后重试。") : "请先手动填写邮箱地址，再执行登录页填入邮箱。";
    switchLuckMailManualMode(fillMessage);
    renderLuckMailOutput(Object.assign({ ok: false, stage: "manual_email_required", manual_mode: !hasLuckMailToken(), error: hasLuckMailToken() ? "token email is required" : "email is required" }, emailReady?.data ? { token_lookup: emailReady.data } : {}));
    return;
  }

  var originalText = controlButton?.textContent || "登录页填入邮箱";
  var startedAt = performance.now();
  if (controlButton) controlButton.disabled = true;
  if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = true;
  if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = true;
  setText(controlButton, "提交邮箱...");
  if (luckmailBadge) { setText(luckmailBadge, "提交邮箱"); luckmailBadge.className = "badge neutral"; }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailEmailAddress, email);
  setText(luckmailVerificationCode, "-");
  setText(luckmailMailMeta, "正在监控无痕窗口，填入邮箱后点击继续按钮，并等待验证码输入页面...");
  if (hasLuckMailToken()) {
    switchLuckMailAutoMode("邮箱提交后将自动等待并填写验证码");
  } else {
    setLuckMailMode("manual", "邮箱提交后需要手动填写验证码");
  }
  announceLuckMailScene("email_fill_started", "正在填写邮箱并等待验证码页面");
  renderLuckMailOutput({ stage: "login_email_fill_started" });

  try {
    var emailSubmitStartedUnixMS = Date.now();
    var resp = await fetch("/api/login/email-fill", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify({ email: email, timeout_s: 300 }),
    });
    var data = await resp.json();
    if (data.email_filled || data.clicked_continue || data.verification_ready) {
      luckMailVerificationSinceUnixMS = Math.max(0, emailSubmitStartedUnixMS - 2000);
    }
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, (data.elapsed_ms || elapsed) + "ms");
    var successMessage = data.verification_ready ? "已提交邮箱并进入验证码输入页面。" : (data.clicked_continue ? "已点击继续按钮，正在等待验证码页面。" : (data.email_mode_switched ? "已切换为电子邮箱方式并填入邮箱。" : "已在登录或注册页面填入邮箱。"));
    var failureMessage =
      data.operation_timed_out ? "页面提示 Operation timed out，建议稍后重试或检查网络/代理后再次提交邮箱。" :
      (data.stage === "login_email_input_not_ready_after_surface" ? "已看到登录框，系统已延长等待邮箱输入框，但输入框仍未出现；建议检查登录页网络加载状态。" : (data.error || "登录页邮箱提交失败"));
    setText(luckmailMailMeta, data.ok ? successMessage : failureMessage);
    renderLuckMailOutput(data);
    if (luckmailBadge) {
      setText(luckmailBadge, data.ok ? (data.verification_ready ? "待验证码" : "已提交") : (data.operation_timed_out ? "提交超时" : "提交失败"));
      luckmailBadge.className = data.ok ? "badge" : "badge error";
    }
    if (data.ok && data.verification_ready) {
      announceLuckMailScene("email_verification_ready", "邮箱已提交，验证码页面已出现");
      if (!hasLuckMailToken()) {
        switchLuckMailManualMode("验证码页面已出现，请手动输入收到的验证码后点击手动填验证码。");
        return data;
      }
      return await waitTokenCodeAndFillLoginCode(options?.codeRetry || {});
    } else if (data.ok) {
      announceLuckMailScene("email_submitted", successMessage);
    } else if (data.operation_timed_out) {
      announceLuckMailScene("email_operation_timed_out", "页面提示提交超时，请检查网络或代理后重试");
    } else {
      announceLuckMailScene("email_fill_failed", data.error || "登录页邮箱提交失败");
    }
  } catch (error) {
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, elapsed + "ms");
    setText(luckmailMailMeta, error.message || "登录页邮箱填入请求失败");
    announceLuckMailScene("email_fill_error", error.message || "登录页邮箱填入请求失败");
    renderLuckMailOutput({ ok: false, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
  } finally {
    if (controlButton) controlButton.disabled = false;
    if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = false;
    if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = false;
    setText(controlButton, originalText || "登录页填入邮箱");
  }
}

async function runLuckMailTokenCode() {
  if (!luckmailTokenCodeBtn) return;
  var originalText = luckmailTokenCodeBtn.textContent;
  var startedAt = performance.now();
  latestLuckMailCode = "";
  luckmailTokenCodeBtn.disabled = true;
  setWorkflowButtonState(luckmailTokenCodeBtn, "running");
  if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = true;
  if (luckmailCopyCodeBtn) luckmailCopyCodeBtn.disabled = true;
  setText(luckmailTokenCodeBtn, "等待已购邮箱中...");
  if (luckmailBadge) { setText(luckmailBadge, "等待中"); luckmailBadge.className = "badge neutral"; }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailEmailAddress, "-");
  setText(luckmailVerificationCode, "-");
  if (!hasLuckMailToken()) {
    var tokenMessage = hasLuckMailTokenInput() ? "LuckMail Token 格式不正确，验证码轮询需要以 tok_ 开头的已购邮箱 Token。" : "未填写 LuckMail Token，请手动填写邮箱地址和验证码。";
    switchLuckMailManualMode(tokenMessage);
    setText(luckmailLatency, Math.round(performance.now() - startedAt) + "ms");
    renderLuckMailOutput({ ok: false, stage: "luckmail_manual_mode", manual_mode: true, error: hasLuckMailTokenInput() ? "luckmail_token invalid format" : "luckmail_token is required" });
    setWorkflowButtonState(luckmailTokenCodeBtn, "error");
    luckmailTokenCodeBtn.disabled = false;
    if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = false;
    setText(luckmailTokenCodeBtn, originalText || "等待已购邮箱验证码");
    return;
  }
  setText(luckmailMailMeta, "正在通过已购邮箱 Token 轮询验证码...");
  announceLuckMailScene("token_code_waiting", "正在等待已购邮箱验证码");
  renderLuckMailOutput({ stage: "luckmail_token_waiting" });

  try {
    var data = await fetchLuckMailTokenCodeWithRecovery({ retryableAttempts: 3 });
    renderLuckMailData(data, Math.round(performance.now() - startedAt));
    if (!data.ok) {
      setWorkflowButtonState(luckmailTokenCodeBtn, "error");
      if (isLuckMailTokenCodeRetryable(data)) {
        setText(luckmailMailMeta, "LuckMail 验证码等待被连续中断，请保持页面不刷新后再次点击 LuckMail 接码。");
        announceLuckMailScene("token_code_retryable_failed", "验证码等待连续中断，请再次点击 LuckMail 接码");
        renderLuckMailOutput(Object.assign({}, data, { auto_retry_exhausted: true }));
      } else {
        switchLuckMailManualMode(tokenFallbackMessage(data));
        renderLuckMailOutput(Object.assign({}, data, { manual_mode: true }));
      }
    } else {
      setWorkflowButtonState(luckmailTokenCodeBtn, "done");
    }
  } catch (error) {
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, elapsed + "ms");
    setText(luckmailMailMeta, error.message || "LuckMail 已购邮箱请求失败");
    announceLuckMailScene("token_code_error", error.message || "LuckMail 已购邮箱请求失败");
    renderLuckMailOutput({ ok: false, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
    setWorkflowButtonState(luckmailTokenCodeBtn, "error");
  } finally {
    luckmailTokenCodeBtn.disabled = false;
    if (luckmailTokenMailsBtn) luckmailTokenMailsBtn.disabled = false;
    setText(luckmailTokenCodeBtn, originalText || "等待已购邮箱验证码");
  }
}

async function runLuckMailTokenMails() {
  if (!luckmailTokenMailsBtn) return;
  var originalText = luckmailTokenMailsBtn.textContent;
  var startedAt = performance.now();
  luckmailTokenMailsBtn.disabled = true;
  if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = true;
  setText(luckmailTokenMailsBtn, "查询邮件中...");
  if (luckmailBadge) { setText(luckmailBadge, "查询中"); luckmailBadge.className = "badge neutral"; }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailMailMeta, "正在查询已购邮箱邮件列表...");
  if (!hasLuckMailToken()) {
    var tokenMessage = hasLuckMailTokenInput() ? "LuckMail Token 格式不正确，邮件查询需要以 tok_ 开头的已购邮箱 Token。" : "未填写 LuckMail Token，无法自动查询邮件列表，请手动填写邮箱地址和验证码。";
    switchLuckMailManualMode(tokenMessage);
    setText(luckmailLatency, Math.round(performance.now() - startedAt) + "ms");
    renderLuckMailMails({ ok: false, error: tokenMessage, mails: [] });
    renderLuckMailOutput({ ok: false, stage: "luckmail_manual_mode", manual_mode: true, error: hasLuckMailTokenInput() ? "luckmail_token invalid format" : "luckmail_token is required" });
    luckmailTokenMailsBtn.disabled = false;
    if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = false;
    setText(luckmailTokenMailsBtn, originalText || "查询邮件列表");
    return;
  }
  announceLuckMailScene("token_mails_loading", "正在查询邮件列表");
  renderLuckMailOutput({ stage: "luckmail_token_mails_loading" });

  try {
    var resp = await fetch("/api/luckmail/token-mails", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(luckMailTokenPayload()),
    });
    var data = await resp.json();
    if (data.email_address && hasLuckMailToken()) {
      applyLuckMailPurchase({ email_address: data.email_address, luckmail_token: (luckmailToken?.value || "").trim() });
    }
    var elapsed = Math.round(performance.now() - startedAt);
    renderLuckMailMails(data);
    setText(luckmailEmailAddress, data.email_address || "-");
    setText(luckmailLatency, (data.elapsed_ms || elapsed) + "ms");
    setText(luckmailMailMeta, data.ok ? ("邮件数：" + ((data.mails || []).length) + (data.warranty_until ? " · 保修至：" + data.warranty_until : "")) : tokenFallbackMessage(data));
    renderLuckMailOutput(data.ok ? data : Object.assign({}, data, { manual_mode: true }));
    if (luckmailBadge) {
      setText(luckmailBadge, data.ok ? "自动模式" : "手动模式");
      luckmailBadge.className = data.ok ? "badge" : "badge neutral";
    }
    if (data.ok) {
      switchLuckMailAutoMode("邮件列表已查询，可继续自动等待验证码");
    } else {
      switchLuckMailManualMode(tokenFallbackMessage(data));
    }
    announceLuckMailScene("token_mails_loaded", data.ok ? ("邮件列表已查询，共 " + ((data.mails || []).length) + " 封") : tokenFallbackMessage(data));
  } catch (error) {
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, elapsed + "ms");
    renderLuckMailMails({ ok: false, error: error.message, mails: [] });
    setText(luckmailMailMeta, error.message || "LuckMail 邮件列表请求失败");
    announceLuckMailScene("token_mails_error", error.message || "LuckMail 邮件列表请求失败");
    renderLuckMailOutput({ ok: false, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
  } finally {
    luckmailTokenMailsBtn.disabled = false;
    if (luckmailTokenCodeBtn) luckmailTokenCodeBtn.disabled = false;
    setText(luckmailTokenMailsBtn, originalText || "查询邮件列表");
  }
}

async function runLuckMailCreateAndWait() {
  if (!luckmailCreateAndWaitBtn) return;
  var originalText = luckmailCreateAndWaitBtn.textContent;
  var startedAt = performance.now();
  latestLuckMailCode = "";
  luckmailCreateAndWaitBtn.disabled = true;
  if (luckmailCopyCodeBtn) luckmailCopyCodeBtn.disabled = true;
  setText(luckmailCreateAndWaitBtn, "等待邮件中...");
  if (luckmailBadge) { setText(luckmailBadge, "等待中"); luckmailBadge.className = "badge neutral"; }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailEmailAddress, "-");
  setText(luckmailVerificationCode, "-");
  setText(luckmailMailMeta, "正在创建 LuckMail 订单并轮询验证码...");
  announceLuckMailScene("temp_order_waiting", "正在创建临时接码订单并等待验证码");
  renderLuckMailOutput({ stage: "luckmail_waiting" });

  var payload = Object.assign({
    project_code: (luckmailProjectCode?.value || "openai").trim(),
    email_type: (luckmailEmailType?.value || "ms_graph").trim(),
    domain: (luckmailDomain?.value || "").trim(),
    specified_email: (luckmailSpecifiedEmail?.value || "").trim(),
    timeout_s: numberInputValue(luckmailTimeoutS, 300),
    interval_s: numberInputValue(luckmailIntervalS, 3),
  }, luckMailAPIContextPayload());

  try {
    var resp = await fetch("/api/luckmail/create-and-wait", {
      method: "POST",
      headers: buildRequestHeaders(),
      body: JSON.stringify(payload),
    });
    var data = await resp.json();
    var elapsed = Math.round(performance.now() - startedAt);
    latestLuckMailCode = data.verification_code || "";
    setText(luckmailEmailAddress, data.email_address || "-");
    setText(luckmailVerificationCode, latestLuckMailCode || "-");
    setText(luckmailLatency, (data.elapsed_ms || elapsed) + "ms");
    setText(luckmailMailMeta, [data.mail_from, data.mail_subject].filter(Boolean).join(" · ") || (data.error || data.status || "-"));
    renderLuckMailOutput(data);
    if (luckmailCopyCodeBtn) luckmailCopyCodeBtn.disabled = !latestLuckMailCode;
    if (luckmailBadge) {
      setText(luckmailBadge, data.ok ? "已收到" : "未收到");
      luckmailBadge.className = data.ok ? "badge" : "badge error";
    }
    announceLuckMailScene("temp_order_done", latestLuckMailCode ? "验证码已收到" : (data.error || "临时接码订单未收到验证码"), !!latestLuckMailCode);
  } catch (error) {
    var elapsed = Math.round(performance.now() - startedAt);
    setText(luckmailLatency, elapsed + "ms");
    setText(luckmailMailMeta, error.message || "LuckMail 请求失败");
    announceLuckMailScene("temp_order_error", error.message || "LuckMail 请求失败");
    renderLuckMailOutput({ ok: false, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "错误"); luckmailBadge.className = "badge error"; }
  } finally {
    luckmailCreateAndWaitBtn.disabled = false;
    setText(luckmailCreateAndWaitBtn, originalText || "创建并等待验证码");
  }
}

async function runLuckMailManualCodeFill() {
  var code = manualLuckMailCodeValue();
  if (!code) {
    if (luckmailResult) luckmailResult.hidden = false;
    switchLuckMailManualMode("请先填写手动验证码，再提交到当前验证码页面。");
    renderLuckMailOutput({ ok: false, stage: "manual_code_required", manual_mode: true, error: "code is required" });
    return;
  }
  var originalText = luckmailManualCodeFillBtn?.textContent || "手动填验证码";
  if (luckmailManualCodeFillBtn) {
    luckmailManualCodeFillBtn.disabled = true;
    setText(luckmailManualCodeFillBtn, "填验证码...");
  }
  if (luckmailResult) luckmailResult.hidden = false;
  setText(luckmailVerificationCode, code);
  switchLuckMailManualMode("正在把手动验证码填入当前验证码页面。");
  try {
    await runLuckMailCodeFill(code);
  } catch (error) {
    setText(luckmailMailMeta, error.message || "手动验证码填入失败");
    announceLuckMailScene("manual_code_fill_error", error.message || "手动验证码填入失败");
    renderLuckMailOutput({ ok: false, stage: "manual_code_fill_failed", manual_mode: true, error: error.message });
    if (luckmailBadge) { setText(luckmailBadge, "填码失败"); luckmailBadge.className = "badge error"; }
  } finally {
    if (luckmailManualCodeFillBtn) {
      luckmailManualCodeFillBtn.disabled = false;
      setText(luckmailManualCodeFillBtn, originalText);
    }
  }
}

luckmailDeviceLoginGuideBtn?.addEventListener("click", runLuckMailDeviceLoginGuide);
luckmailLoadPurchaseBtn?.addEventListener("click", runLuckMailLoadPurchase);
luckmailLoginEmailFillBtn?.addEventListener("click", runLuckMailLoginEmailFill);
luckmailManualCodeFillBtn?.addEventListener("click", runLuckMailManualCodeFill);
luckmailTokenCodeBtn?.addEventListener("click", runLuckMailTokenCode);
luckmailTokenMailsBtn?.addEventListener("click", runLuckMailTokenMails);
luckmailShareCodeLinkBtn?.addEventListener("click", copyLuckMailShareCodeLink);
luckmailCreateAndWaitBtn?.addEventListener("click", runLuckMailCreateAndWait);
luckmailCopyCodeBtn?.addEventListener("click", async function () {
  if (!latestLuckMailCode) return;
  var copied = await copyText(latestLuckMailCode);
  setText(luckmailCopyCodeBtn, copied ? "已复制" : "复制失败");
  window.setTimeout(function () { setText(luckmailCopyCodeBtn, "复制验证码"); }, 1200);
});

// =================================
// GoPay 流程监控
// =================================
const gopayMonitorBtn = document.querySelector("#gopayMonitorBtn");
const gopayMonitor = document.querySelector("#gopayMonitor");
const monitorBadge = document.querySelector("#monitorBadge");
const monitorSummary = document.querySelector("#monitorSummary");
const monitorLog = document.querySelector("#monitorLog");
const monitorOutput = document.querySelector("#monitorOutput");
const monitorNetCount = document.querySelector("#monitorNetCount");
const monitorPageCount = document.querySelector("#monitorPageCount");
const monitorAPICount = document.querySelector("#monitorAPICount");
const monitorErrorCount = document.querySelector("#monitorErrorCount");

let gopayMonitorAbortController = null;
let gopayMonitorRunning = false;
let sessionMonitorAbortController = null;

const DOMAIN_ICONS = {
  Network:  "NET",
  Page:     "PG",
  Console:  "JS",
  Error:    "ERR",
  Log:      "LOG",
};

const DOMAIN_COLORS = {
  Network:  "#2563eb",
  Page:     "#059669",
  Console:  "#d97706",
  Error:    "#dc2626",
  Log:      "#6b7280",
};

function clearMonitor() {
  if (monitorLog) monitorLog.innerHTML = '<div class="monitor-empty">等待数据...</div>';
  if (monitorSummary) monitorSummary.hidden = true;
  if (monitorOutput) { monitorOutput.hidden = true; monitorOutput.textContent = "{}"; }
  if (monitorNetCount) monitorNetCount.textContent = "0";
  if (monitorPageCount) monitorPageCount.textContent = "0";
  if (monitorAPICount) monitorAPICount.textContent = "0";
  if (monitorErrorCount) monitorErrorCount.textContent = "0";
}

function setMonitorSummary(summary) {
  if (!summary) return;
  if (monitorSummary) monitorSummary.hidden = false;
  if (monitorNetCount) monitorNetCount.textContent = summary.network_calls || 0;
  if (monitorPageCount) monitorPageCount.textContent = summary.page_navigations || 0;
  if (monitorAPICount) {
    monitorAPICount.textContent =
      (summary.openai_api_calls || 0) +
      (summary.stripe_api_calls || 0) +
      (summary.snap_api_calls || 0) +
      (summary.gopay_api_calls || 0);
  }
  if (monitorErrorCount) monitorErrorCount.textContent = summary.errors || 0;
}

async function startGopayMonitorStream(durationS) {
  gopayMonitorAbortController = new AbortController();
  gopayMonitorRunning = true;

  var resp = await fetch("/api/gopay/monitor", {
    method: "POST",
    headers: buildRequestHeaders(),
    body: JSON.stringify({ duration_s: durationS, stream: true }),
    signal: gopayMonitorAbortController.signal,
  });

  if (!resp.ok || !resp.body) {
    throw new Error("监控启动失败");
  }

  var reader = resp.body.getReader();
  var decoder = new TextDecoder();
  var buffer = "";
  var fullOutput = [];

  while (true) {
    var chunk = await reader.read();
    if (chunk.done) break;
    buffer += decoder.decode(chunk.value, { stream: true });
    var lines = buffer.split("\n");
    buffer = lines.pop() || "";

    for (var i = 0; i < lines.length; i++) {
      var line = lines[i].trim();
      if (!line) continue;
      var msg = JSON.parse(line);
      fullOutput.push(msg);

      if (monitorOutput) {
        monitorOutput.hidden = false;
        monitorOutput.textContent = JSON.stringify(fullOutput, null, 2);
      }

      if (msg.type === "started") {
        appendMonitorEvent({ domain: "Log", method: "monitor-started", summary: (msg.targets || []).join(" | "), ts: Date.now() });
      } else if (msg.type === "event" && msg.event) {
        appendMonitorEvent(msg.event);
      } else if ((msg.type === "summary" || msg.type === "done") && msg.summary) {
        setMonitorSummary(msg.summary);
      } else if (msg.type === "error") {
        appendMonitorEvent({ domain: "Error", method: "monitor-error", summary: msg.error || "未知错误", ts: Date.now() });
      }
    }
  }
}

function appendMonitorEvent(evt) {
  if (!monitorLog) return;
  if (monitorLog.querySelector(".monitor-empty")) {
    monitorLog.innerHTML = "";
  }

  var icon = DOMAIN_ICONS[evt.domain] || "\u25CF";
  var color = DOMAIN_COLORS[evt.domain] || "#6b7280";
  var time = new Date(evt.ts).toLocaleTimeString();

  var row = document.createElement("div");
  row.className = "monitor-row";
  row.style.borderLeftColor = color;
  row.innerHTML =
    '<span class="monitor-icon">' + icon + '</span>' +
    '<span class="monitor-time">' + time + '</span>' +
    '<span class="monitor-method" style="color:' + color + '">[' + evt.method + ']</span>' +
    '<span class="monitor-text">' + escapeHTML(evt.summary) + '</span>';
  monitorLog.appendChild(row);

  var MAX_ROWS = 300;
  while (monitorLog.children.length > MAX_ROWS) {
    monitorLog.removeChild(monitorLog.firstChild);
  }
  monitorLog.scrollTop = monitorLog.scrollHeight;
}

function escapeHTML(str) {
  return String(str).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
}

gopayMonitorBtn?.addEventListener("click", async function () {
  if (!gopayMonitor) return;

  if (gopayPaymentFlowRunning) {
    showAutoFillNotice("GoPay 付款进行中", "提示", "一键绑定付款流程运行期间暂不启动或停止流程监控，避免中断付款状态。", "", "");
    return;
  }

  if (gopayMonitorRunning) {
    if (gopayMonitorAbortController) {
      gopayMonitorAbortController.abort();
    }
    gopayMonitorRunning = false;
    gopayMonitorBtn.textContent = "开始流程监控";
    if (monitorBadge) { monitorBadge.textContent = "已停止"; monitorBadge.className = "badge neutral"; }
    appendMonitorEvent({ domain: "Log", method: "monitor-stop", summary: "用户已停止监控", ts: Date.now() });
    return;
  }

  if (sessionMonitorAbortController) {
    sessionMonitorAbortController.abort();
    sessionMonitorAbortController = null;
    if (monitorBadge) { monitorBadge.textContent = "Session 监控已停止"; monitorBadge.className = "badge neutral"; }
    appendMonitorEvent({ domain: "Log", method: "session-monitor-stop", summary: "用户已停止 Session 监控", ts: Date.now() });
    return;
  }

  gopayMonitor.hidden = false;
  gopayMonitorBtn.textContent = "停止监控";
  clearMonitor();

  if (monitorBadge) { monitorBadge.textContent = "监控中"; monitorBadge.className = "badge"; }
  if (monitorSummary) monitorSummary.hidden = true;

  var durationS = 25;

  try {
    await startGopayMonitorStream(durationS);
    if (monitorBadge) { monitorBadge.textContent = "完成"; monitorBadge.className = "badge"; }
  } catch (err) {
    if (err && err.name === "AbortError") {
      if (monitorBadge) { monitorBadge.textContent = "已停止"; monitorBadge.className = "badge neutral"; }
    } else {
      if (monitorBadge) { monitorBadge.textContent = "错误"; monitorBadge.className = "badge error"; }
      appendMonitorEvent({ domain: "Error", method: "monitor-error", summary: err.message, ts: Date.now() });
    }
  } finally {
    gopayMonitorRunning = false;
    gopayMonitorAbortController = null;
    gopayMonitorBtn.textContent = "开始流程监控";
  }
});

bindWechatGroupCard();
void checkHealth();
void bootstrapPlusSubscribeSessionTriggerWatcher();
