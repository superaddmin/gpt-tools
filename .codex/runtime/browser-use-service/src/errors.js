/**
 * 统一业务错误对象。
 */
export class BrowserUseError extends Error {
  /**
   * 创建业务错误。
   * @param {string} code
   * @param {string} message
   * @param {{ stage?: string, status?: number, details?: object, cause?: unknown }} [options]
   */
  constructor(code, message, options = {}) {
    super(message);
    this.name = "BrowserUseError";
    this.code = code;
    this.stage = options.stage || "unknown";
    this.status = options.status || 500;
    this.details = options.details || {};
    this.cause = options.cause;
  }
}

/**
 * 将未知异常归一化为 BrowserUseError。
 * @param {unknown} error
 * @param {string} stage
 * @returns {BrowserUseError}
 */
export function normalizeError(error, stage) {
  if (error instanceof BrowserUseError) {
    return error;
  }

  if (error instanceof Error) {
    return new BrowserUseError("UNEXPECTED_ERROR", error.message, {
      stage,
      cause: error,
      details: { stack: error.stack },
    });
  }

  return new BrowserUseError("UNKNOWN_THROWABLE", "发生未知异常", {
    stage,
    details: { value: error },
  });
}

/**
 * 判断错误是否允许重试。
 * @param {BrowserUseError} error
 * @returns {boolean}
 */
export function isRetryableError(error) {
  return [
    "NAVIGATION_TIMEOUT",
    "ELEMENT_NOT_FOUND",
    "WAIT_CONDITION_TIMEOUT",
    "TRANSIENT_ACTION_FAILURE",
    "NETWORK_FAILURE",
  ].includes(error.code);
}
