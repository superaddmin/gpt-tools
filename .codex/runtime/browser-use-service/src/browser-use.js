import { chromium, firefox, webkit } from "playwright";
import { BrowserUseError, isRetryableError, normalizeError } from "./errors.js";
import { safeExtractPageData } from "./extractor.js";
import { createLogger, redactValue } from "./logger.js";

const BROWSERS = { chromium, firefox, webkit };

/**
 * 规范化任务输入。
 * @param {object} input
 * @returns {object}
 */
export function normalizeTaskInput(input = {}) {
  const url = typeof input.url === "string" ? input.url.trim() : "";
  if (!url) {
    throw new BrowserUseError("INVALID_INPUT", "缺少必填字段 url", {
      stage: "input",
      status: 400,
    });
  }

  return {
    task: typeof input.task === "string" && input.task.trim() ? input.task.trim() : "browser-use task",
    url,
    browser: ["chromium", "firefox", "webkit"].includes(input.browser) ? input.browser : "chromium",
    headless: input.headless !== false,
    timeoutMs: Number.isFinite(input.timeoutMs) ? Number(input.timeoutMs) : 30000,
    actionTimeoutMs: Number.isFinite(input.actionTimeoutMs) ? Number(input.actionTimeoutMs) : 15000,
    submit: Boolean(input.submit),
    formData: isRecord(input.formData) ? input.formData : {},
    fieldHints: isRecord(input.fieldHints) ? input.fieldHints : {},
    waitFor: isRecord(input.waitFor) ? input.waitFor : {},
    extract: input.extract || "full",
    logLevel: typeof input.logLevel === "string" ? input.logLevel : "info",
    retryCount: Number.isFinite(input.retryCount) ? Number(input.retryCount) : 2,
    screenshotOnError: input.screenshotOnError !== false,
    outputSchema: isRecord(input.outputSchema) ? input.outputSchema : null,
    headers: isRecord(input.headers) ? input.headers : {},
    storageState: typeof input.storageState === "string" ? input.storageState : undefined,
  };
}

/**
 * 执行 browser-use 自动化任务。
 * @param {object} rawInput
 * @returns {Promise<object>}
 */
export async function runBrowserUse(rawInput) {
  const input = normalizeTaskInput(rawInput);
  const logger = createLogger({ level: input.logLevel });
  const startedAt = Date.now();

  logger.info("session.start", {
    task: input.task,
    url: input.url,
    browser: input.browser,
    headless: input.headless,
  });

  const warnings = [];
  const errors = [];
  let latestPage;

  try {
    const result = await withRetry(input.retryCount, logger, async (attempt) => {
      /** @type {import('playwright').Browser | undefined} */
      let browser;
      /** @type {import('playwright').BrowserContext | undefined} */
      let context;
      /** @type {import('playwright').Page | undefined} */
      let page;

      try {
        logger.info("session.attempt", { attempt });
        browser = await launchBrowser(input, logger);
        const contextOptions = {
          extraHTTPHeaders: input.headers,
        };
        if (input.storageState) {
          contextOptions.storageState = input.storageState;
        }

        context = await browser.newContext(contextOptions);
        page = await context.newPage();
        latestPage = page;
        await attachPageTelemetry(page, logger);
        page.setDefaultTimeout(input.actionTimeoutMs);
        page.setDefaultNavigationTimeout(input.timeoutMs);

        await navigateToPage(page, input, logger);
        const formScan = await scanPageForms(page, logger);
        const fillResult = await fillForms(page, input, logger, warnings);
        const submitResult = input.submit ? await submitForm(page, input, logger, warnings) : { submitted: false };
        const data = await safeExtractPageData(page);

        return buildSuccessResult({
          input,
          data: {
            ...data,
            formScan,
            fillResult,
            submitResult,
          },
          warnings,
          logs: logger.entries,
          startedAt,
        });
      } finally {
        await closeSafely(page, context, browser, logger);
      }
    });

    logger.info("session.end", {
      status: result.status,
      durationMs: Date.now() - startedAt,
      warnings: warnings.length,
      errors: errors.length,
    });

    result.logs = logger.entries;
    return result;
  } catch (error) {
    const normalized = normalizeError(error, "run");
    errors.push({
      code: normalized.code,
      message: normalized.message,
      stage: normalized.stage,
      details: normalized.details,
    });

    logger.error("error", {
      code: normalized.code,
      stage: normalized.stage,
      message: normalized.message,
    });

    const screenshotPath = input.screenshotOnError && latestPage ? await captureFailureScreenshot(latestPage, logger) : null;
    const failedResult = {
      status: "failed",
      task: input.task,
      url: input.url,
      submitted: false,
      summary: normalized.message,
      data: screenshotPath ? { screenshotPath } : {},
      logs: logger.entries,
      warnings,
      errors,
      durationMs: Date.now() - startedAt,
    };

    logger.info("session.end", {
      status: failedResult.status,
      durationMs: failedResult.durationMs,
      warnings: warnings.length,
      errors: errors.length,
    });

    return failedResult;
  }
}

/**
 * 启动浏览器实例。
 * @param {object} input
 * @param {ReturnType<typeof createLogger>} logger
 * @returns {Promise<import('playwright').Browser>}
 */
async function launchBrowser(input, logger) {
  const factory = BROWSERS[input.browser];
  if (!factory) {
    throw new BrowserUseError("UNSUPPORTED_BROWSER", `不支持的浏览器类型: ${input.browser}`, {
      stage: "launch",
      status: 400,
    });
  }

  try {
    logger.info("browser.launch", { browser: input.browser, headless: input.headless });
    return await factory.launch({ headless: input.headless });
  } catch (error) {
    throw new BrowserUseError("BROWSER_LAUNCH_FAILED", "浏览器启动失败", {
      stage: "launch",
      cause: error,
    });
  }
}

/**
 * 为页面注册诊断监听器。
 * @param {import('playwright').Page} page
 * @param {ReturnType<typeof createLogger>} logger
 * @returns {Promise<void>}
 */
async function attachPageTelemetry(page, logger) {
  page.on("console", (message) => {
    logger.debug("page.console", {
      type: message.type(),
      text: message.text(),
    });
  });

  page.on("pageerror", (error) => {
    logger.warning("page.error", {
      message: error.message,
    });
  });

  page.on("requestfailed", (request) => {
    logger.warning("network.requestfailed", {
      url: request.url(),
      method: request.method(),
      failure: request.failure()?.errorText || "unknown",
    });
  });
}

/**
 * 导航到目标页面并等待稳定态。
 * @param {import('playwright').Page} page
 * @param {object} input
 * @param {ReturnType<typeof createLogger>} logger
 * @returns {Promise<void>}
 */
async function navigateToPage(page, input, logger) {
  logger.info("navigation.start", { url: input.url });
  try {
    const response = await page.goto(input.url, {
      waitUntil: "domcontentloaded",
      timeout: input.timeoutMs,
    });
    await page.waitForLoadState("load", { timeout: input.timeoutMs }).catch(() => null);

    logger.info("navigation.done", {
      url: page.url(),
      status: response?.status() || null,
    });

    await waitForConditions(page, input.waitFor, input.timeoutMs, logger);
  } catch (error) {
    throw new BrowserUseError("NAVIGATION_TIMEOUT", "页面加载或等待稳定态失败", {
      stage: "navigation",
      cause: error,
    });
  }
}

/**
 * 根据任务条件等待页面状态。
 * @param {import('playwright').Page} page
 * @param {object} waitFor
 * @param {number} timeoutMs
 * @param {ReturnType<typeof createLogger>} logger
 * @returns {Promise<void>}
 */
async function waitForConditions(page, waitFor, timeoutMs, logger) {
  if (waitFor.selector) {
    logger.debug("wait.selector", { selector: waitFor.selector });
    await page.locator(waitFor.selector).first().waitFor({ timeout: timeoutMs });
  }

  if (waitFor.text) {
    logger.debug("wait.text", { text: waitFor.text });
    await page.getByText(waitFor.text, { exact: false }).first().waitFor({ timeout: timeoutMs });
  }

  if (waitFor.urlIncludes) {
    logger.debug("wait.url", { urlIncludes: waitFor.urlIncludes });
    await page.waitForURL(`**${waitFor.urlIncludes}**`, { timeout: timeoutMs });
  }
}

/**
 * 扫描页面表单概况。
 * @param {import('playwright').Page} page
 * @param {ReturnType<typeof createLogger>} logger
 * @returns {Promise<object>}
 */
async function scanPageForms(page, logger) {
  const scan = await page.evaluate(() => ({
    forms: Array.from(document.forms).map((form, index) => ({
      index,
      id: form.id || "",
      name: form.getAttribute("name") || "",
      fields: Array.from(form.elements).map((field) => ({
        tag: field.tagName,
        type: field.getAttribute?.("type") || "",
        name: field.getAttribute?.("name") || "",
        id: field.getAttribute?.("id") || "",
        placeholder: field.getAttribute?.("placeholder") || "",
        ariaLabel: field.getAttribute?.("aria-label") || "",
      })),
    })),
  }));
  logger.info("form.scan", { forms: scan.forms.length });
  return scan;
}

/**
 * 按输入数据填写页面表单。
 * @param {import('playwright').Page} page
 * @param {object} input
 * @param {ReturnType<typeof createLogger>} logger
 * @param {object[]} warnings
 * @returns {Promise<object>}
 */
async function fillForms(page, input, logger, warnings) {
  const results = [];
  for (const [fieldName, rawValue] of Object.entries(input.formData)) {
    const value = rawValue == null ? "" : String(rawValue);
    const locator = await findFieldLocator(page, fieldName, input.fieldHints[fieldName], logger);

    if (!locator) {
      const warning = { code: "FIELD_NOT_FOUND", field: fieldName, message: `未找到字段 ${fieldName}` };
      warnings.push(warning);
      logger.warning("form.fill.missing", warning);
      results.push({ field: fieldName, status: "missing" });
      continue;
    }

    try {
      const tagName = await locator.evaluate((node) => node.tagName.toLowerCase());
      const type = await locator.evaluate((node) => node.getAttribute("type") || "text");
      logger.info("form.fill", {
        field: fieldName,
        tagName,
        type,
        valuePreview: redactValue(value),
      });

      await fillLocator(locator, value, tagName, type);
      const verified = await verifyLocatorValue(locator, value, type);
      results.push({ field: fieldName, status: verified ? "filled" : "unverified", type });

      if (!verified) {
        const warning = { code: "FIELD_UNVERIFIED", field: fieldName, message: `字段 ${fieldName} 写入后未能确认值` };
        warnings.push(warning);
        logger.warning("form.fill.unverified", warning);
      }
    } catch (error) {
      throw new BrowserUseError("TRANSIENT_ACTION_FAILURE", `填写字段失败: ${fieldName}`, {
        stage: "fill",
        cause: error,
        details: { fieldName },
      });
    }
  }

  return { fields: results };
}

/**
 * 提交页面表单并等待反馈。
 * @param {import('playwright').Page} page
 * @param {object} input
 * @param {ReturnType<typeof createLogger>} logger
 * @param {object[]} warnings
 * @returns {Promise<object>}
 */
async function submitForm(page, input, logger, warnings) {
  const selectorCandidates = [
    'form button[type="submit"]',
    'form input[type="submit"]',
    'button[type="submit"]',
    'input[type="submit"]',
    'button:has-text("提交")',
    'button:has-text("Submit")',
  ];

  for (const selector of selectorCandidates) {
    const locator = page.locator(selector).first();
    if (await locator.count()) {
      logger.info("form.submit", { selector });
      await locator.click({ timeout: input.actionTimeoutMs });
      await waitForConditions(page, input.waitFor, Math.min(input.timeoutMs, 10000), logger).catch((error) => {
        warnings.push({ code: "SUBMIT_WAIT_UNCERTAIN", message: "提交后未捕获到明确完成信号" });
        logger.warning("form.submit.uncertain", { message: error.message });
      });
      return { submitted: true, selector, finalUrl: page.url() };
    }
  }

  throw new BrowserUseError("ELEMENT_NOT_FOUND", "未找到可提交的表单按钮", {
    stage: "submit",
  });
}

/**
 * 查找表单字段定位器。
 * @param {import('playwright').Page} page
 * @param {string} fieldName
 * @param {object | undefined} hint
 * @param {ReturnType<typeof createLogger>} logger
 * @returns {Promise<import('playwright').Locator | null>}
 */
async function findFieldLocator(page, fieldName, hint, logger) {
  const candidates = [];

  if (hint && typeof hint.selector === "string" && hint.selector.trim()) {
    candidates.push(page.locator(hint.selector).first());
  }

  candidates.push(page.getByLabel(fieldName, { exact: false }).first());
  candidates.push(page.getByPlaceholder(fieldName, { exact: false }).first());
  candidates.push(page.locator(`[name="${escapeAttribute(fieldName)}"]`).first());
  candidates.push(page.locator(`#${escapeAttribute(fieldName)}`).first());
  candidates.push(page.locator(`[aria-label="${escapeAttribute(fieldName)}"]`).first());

  if (hint && typeof hint === "object") {
    for (const key of ["label", "placeholder", "name", "id", "ariaLabel"]) {
      const value = hint[key];
      if (typeof value !== "string" || !value.trim()) {
        continue;
      }
      if (key === "label") {
        candidates.push(page.getByLabel(value, { exact: false }).first());
      }
      if (key === "placeholder") {
        candidates.push(page.getByPlaceholder(value, { exact: false }).first());
      }
      if (key === "name") {
        candidates.push(page.locator(`[name="${escapeAttribute(value)}"]`).first());
      }
      if (key === "id") {
        candidates.push(page.locator(`#${escapeAttribute(value)}`).first());
      }
      if (key === "ariaLabel") {
        candidates.push(page.locator(`[aria-label="${escapeAttribute(value)}"]`).first());
      }
    }
  }

  for (const locator of candidates) {
    try {
      if (await locator.count()) {
        logger.debug("form.locator.found", { field: fieldName });
        return locator;
      }
    } catch (_error) {
      logger.debug("form.locator.skip", { field: fieldName });
    }
  }

  return null;
}

/**
 * 根据控件类型填写值。
 * @param {import('playwright').Locator} locator
 * @param {string} value
 * @param {string} tagName
 * @param {string} type
 * @returns {Promise<void>}
 */
async function fillLocator(locator, value, tagName, type) {
  if (tagName === "select") {
    await locator.selectOption({ label: value }).catch(async () => {
      await locator.selectOption(value);
    });
    return;
  }

  if (type === "checkbox" || type === "radio") {
    if (value === "false" || value === "0" || value === "no") {
      await locator.uncheck();
    } else {
      await locator.check();
    }
    return;
  }

  const contentEditable = await locator.evaluate((node) => node.getAttribute("contenteditable") === "true");
  if (contentEditable) {
    await locator.click();
    await locator.press("Control+A").catch(() => null);
    await locator.type(value);
    return;
  }

  await locator.fill(value);
}

/**
 * 校验字段值是否写入成功。
 * @param {import('playwright').Locator} locator
 * @param {string} expectedValue
 * @param {string} type
 * @returns {Promise<boolean>}
 */
async function verifyLocatorValue(locator, expectedValue, type) {
  if (type === "checkbox" || type === "radio") {
    const expectedChecked = !(expectedValue === "false" || expectedValue === "0" || expectedValue === "no");
    return locator.isChecked().then((actual) => actual === expectedChecked);
  }

  const actualValue = await locator.inputValue().catch(async () => {
    return locator.textContent().then((value) => value || "");
  });
  return String(actualValue).trim() === expectedValue.trim();
}

/**
 * 在失败时保存截图。
 * @param {import('playwright').Page} page
 * @param {ReturnType<typeof createLogger>} logger
 * @returns {Promise<string | null>}
 */
async function captureFailureScreenshot(page, logger) {
  try {
    const fileName = `failure-${Date.now()}.png`;
    const filePath = new URL(`../artifacts/${fileName}`, import.meta.url);
    const fs = await import("node:fs/promises");
    await fs.mkdir(new URL("../artifacts/", import.meta.url), { recursive: true });
    await page.screenshot({ path: filePath, fullPage: true });
    logger.info("capture.screenshot", { filePath: filePath.pathname });
    return filePath.pathname;
  } catch (error) {
    logger.warning("capture.screenshot.failed", { message: error instanceof Error ? error.message : String(error) });
    return null;
  }
}

/**
 * 安全关闭浏览器相关资源。
 * @param {import('playwright').Page | undefined} page
 * @param {import('playwright').BrowserContext | undefined} context
 * @param {import('playwright').Browser | undefined} browser
 * @param {ReturnType<typeof createLogger>} logger
 * @returns {Promise<void>}
 */
async function closeSafely(page, context, browser, logger) {
  for (const resource of [page, context, browser]) {
    if (!resource) {
      continue;
    }
    try {
      await resource.close();
    } catch (error) {
      logger.warning("resource.close.failed", {
        message: error instanceof Error ? error.message : String(error),
      });
    }
  }
}

/**
 * 以重试方式运行异步任务。
 * @param {number} retryCount
 * @param {ReturnType<typeof createLogger>} logger
 * @param {(attempt: number) => Promise<object>} operation
 * @returns {Promise<object>}
 */
async function withRetry(retryCount, logger, operation) {
  let attempt = 0;
  let lastError;

  while (attempt <= retryCount) {
    try {
      return await operation(attempt + 1);
    } catch (error) {
      const normalized = normalizeError(error, "retry");
      lastError = normalized;
      logger.warning("retry.failure", {
        attempt: attempt + 1,
        code: normalized.code,
        message: normalized.message,
      });
      if (attempt >= retryCount || !isRetryableError(normalized)) {
        break;
      }
      await delay(Math.min(1000 * (attempt + 1), 3000));
      attempt += 1;
    }
  }

  throw lastError;
}

/**
 * 构建成功结果对象。
 * @param {{ input: object, data: object, warnings: object[], logs: object[], startedAt: number }} params
 * @returns {object}
 */
function buildSuccessResult(params) {
  const { input, data, warnings, logs, startedAt } = params;
  return {
    status: warnings.length ? "partial" : "success",
    task: input.task,
    url: input.url,
    submitted: Boolean(data.submitResult?.submitted),
    summary: warnings.length ? "任务完成，但存在部分警告" : "任务执行成功",
    data,
    logs,
    warnings,
    errors: [],
    durationMs: Date.now() - startedAt,
  };
}

/**
 * 判断值是否为普通对象。
 * @param {unknown} value
 * @returns {value is Record<string, any>}
 */
function isRecord(value) {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

/**
 * 延时等待。
 * @param {number} ms
 * @returns {Promise<void>}
 */
function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

/**
 * 转义属性选择器中的双引号。
 * @param {string} value
 * @returns {string}
 */
function escapeAttribute(value) {
  return String(value).replaceAll('"', '\\"');
}
