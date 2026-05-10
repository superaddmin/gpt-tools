/**
 * 返回 ISO 时间戳。
 * @returns {string}
 */
export function nowIso() {
  return new Date().toISOString();
}

/**
 * 创建结构化日志记录器。
 * @param {{ level?: string, sink?: (entry: object) => void }} options
 * @returns {{ debug: Function, info: Function, warning: Function, error: Function, child: Function, entries: object[] }}
 */
export function createLogger(options = {}) {
  const levels = ["debug", "info", "warning", "error"];
  const currentLevel = levels.includes(options.level) ? options.level : "info";
  const threshold = levels.indexOf(currentLevel);
  const sink = typeof options.sink === "function" ? options.sink : defaultSink;
  const entries = [];

  function write(level, event, detail = {}) {
    const index = levels.indexOf(level);
    const entry = {
      timestamp: nowIso(),
      level,
      event,
      ...detail,
    };

    entries.push(entry);

    if (index >= threshold) {
      sink(entry);
    }

    return entry;
  }

  return {
    entries,
    debug(event, detail) {
      return write("debug", event, detail);
    },
    info(event, detail) {
      return write("info", event, detail);
    },
    warning(event, detail) {
      return write("warning", event, detail);
    },
    error(event, detail) {
      return write("error", event, detail);
    },
    child(baseDetail = {}) {
      return {
        entries,
        debug(event, detail) {
          return write("debug", event, { ...baseDetail, ...(detail || {}) });
        },
        info(event, detail) {
          return write("info", event, { ...baseDetail, ...(detail || {}) });
        },
        warning(event, detail) {
          return write("warning", event, { ...baseDetail, ...(detail || {}) });
        },
        error(event, detail) {
          return write("error", event, { ...baseDetail, ...(detail || {}) });
        },
      };
    },
  };
}

/**
 * 将日志写到标准输出。
 * @param {object} entry
 * @returns {void}
 */
function defaultSink(entry) {
  console.log(JSON.stringify(entry));
}

/**
 * 对敏感值进行脱敏。
 * @param {unknown} value
 * @returns {unknown}
 */
export function redactValue(value) {
  if (typeof value !== "string") {
    return value;
  }

  if (value.length <= 8) {
    return "***";
  }

  return `${value.slice(0, 3)}***${value.slice(-3)}`;
}
