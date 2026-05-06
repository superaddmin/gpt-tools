import http from "node:http";
import { URL } from "node:url";
import { runBrowserUse } from "./browser-use.js";
import { BrowserUseError } from "./errors.js";
import { createLogger } from "./logger.js";

const logger = createLogger({ level: process.env.BROWSER_USE_LOG_LEVEL || "info" });
const port = Number(process.env.BROWSER_USE_PORT || 38765);

/**
 * 启动本地 HTTP 服务。
 * @returns {void}
 */
function startServer() {
  const server = http.createServer(async (request, response) => {
    applyCorsHeaders(response);

    if (request.method === "OPTIONS") {
      response.writeHead(204);
      response.end();
      return;
    }

    try {
      const requestUrl = new URL(request.url || "/", `http://${request.headers.host || `127.0.0.1:${port}`}`);
      logger.info("http.request", {
        method: request.method,
        path: requestUrl.pathname,
      });

      if (request.method === "GET" && requestUrl.pathname === "/health") {
        return writeJson(response, 200, { ok: true, service: "browser-use", port });
      }

      if (request.method === "POST" && requestUrl.pathname === "/run") {
        const body = await readJsonBody(request);
        const result = await runBrowserUse(body);
        return writeJson(response, result.status === "failed" ? 500 : 200, result);
      }

      return writeJson(response, 404, {
        error: "NOT_FOUND",
        message: "未找到请求路径",
      });
    } catch (error) {
      if (error instanceof BrowserUseError) {
        logger.warning("http.business_error", {
          code: error.code,
          stage: error.stage,
          message: error.message,
        });
        return writeJson(response, error.status || 400, {
          error: error.code,
          message: error.message,
          stage: error.stage,
          details: error.details || {},
        });
      }

      logger.error("http.unhandled_error", {
        message: error instanceof Error ? error.message : String(error),
      });
      return writeJson(response, 500, {
        error: "INTERNAL_SERVER_ERROR",
        message: error instanceof Error ? error.message : "未知错误",
      });
    }
  });

  server.listen(port, "127.0.0.1", () => {
    logger.info("server.started", {
      port,
      url: `http://127.0.0.1:${port}`,
    });
  });
}

/**
 * 为响应写入跨域头。
 * @param {import('node:http').ServerResponse} response
 * @returns {void}
 */
function applyCorsHeaders(response) {
  response.setHeader("Access-Control-Allow-Origin", "*");
  response.setHeader("Access-Control-Allow-Methods", "GET,POST,OPTIONS");
  response.setHeader("Access-Control-Allow-Headers", "Content-Type, X-Account-Email");
}

/**
 * 读取 JSON 请求体。
 * @param {import('node:http').IncomingMessage} request
 * @returns {Promise<object>}
 */
async function readJsonBody(request) {
  const chunks = [];
  for await (const chunk of request) {
    chunks.push(Buffer.from(chunk));
  }
  const raw = Buffer.concat(chunks).toString("utf8").trim();
  if (!raw) {
    return {};
  }

  try {
    return JSON.parse(raw);
  } catch (error) {
    throw new BrowserUseError("INVALID_JSON", "请求体不是合法 JSON", {
      stage: "http",
      status: 400,
      cause: error,
    });
  }
}

/**
 * 写入 JSON 响应。
 * @param {import('node:http').ServerResponse} response
 * @param {number} statusCode
 * @param {object} payload
 * @returns {void}
 */
function writeJson(response, statusCode, payload) {
  response.writeHead(statusCode, {
    "Content-Type": "application/json; charset=utf-8",
  });
  response.end(JSON.stringify(payload, null, 2));
}

startServer();
