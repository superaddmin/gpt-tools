const form = document.querySelector("#checkoutForm");
const submitButton = document.querySelector("#submitButton");
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

let latestCheckoutURL = "";

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

function extractAccessToken(rawValue) {
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
  return value;
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

function buildStartPayload() {
  fields.token.value = extractAccessToken(fields.token.value);
  fields.customerEmail.value = randomGmail();
  return {
    token: fields.token.value.trim(),
    entry_point: fields.entryPoint.value.trim(),
    plan_name: fields.planName.value.trim(),
    billing_details: {
      country: fields.country.value.trim(),
      currency: fields.currency.value.trim(),
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
  };
}

async function postJSON(url, payload) {
  const response = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
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

function resetResult() {
  latestCheckoutURL = "";
  setText(statusText, "生成中");
  setText(sessionText, "-");
  setText(hostText, "-");
  setText(finalTransactionText, "请求上游");
  setText(finalSettlementText, "-");
  setText(output, "{}");
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
}

function renderResult(data, elapsedMs, ok) {
  const checkoutURL = data.checkout_url || data.url || "";
  latestCheckoutURL = checkoutURL;

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
}

form?.addEventListener("submit", async (event) => {
  event.preventDefault();
  resetResult();
  submitButton.disabled = true;
  setText(submitButton, "生成中");
  setText(latency, "运行中");

  const startedAt = performance.now();
  try {
    const { response, data } = await postJSON("/api/checkout/start", buildStartPayload());
    renderResult(data, Math.round(performance.now() - startedAt), response.ok);
    if (response.ok && latestCheckoutURL) {
      void copyCurrentCheckoutURL();
    }
  } catch (error) {
    renderResult({ error: error.message }, Math.round(performance.now() - startedAt), false);
  } finally {
    submitButton.disabled = false;
    setText(submitButton, "生成支付链接");
  }
});

copyCheckoutLinkButton?.addEventListener("click", () => {
  void copyCurrentCheckoutURL();
});

void checkHealth();
