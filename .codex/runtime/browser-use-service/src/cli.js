import process from "node:process";
import { runBrowserUse } from "./browser-use.js";

/**
 * CLI 主入口。
 * @returns {Promise<void>}
 */
async function main() {
  try {
    const input = await readTaskInput();
    const result = await runBrowserUse(input);
    process.stdout.write(`${JSON.stringify(result, null, 2)}\n`);
    process.exitCode = result.status === "failed" ? 1 : 0;
  } catch (error) {
    const payload = {
      status: "failed",
      summary: error instanceof Error ? error.message : "未知错误",
      errors: [
        {
          message: error instanceof Error ? error.message : String(error),
        },
      ],
    };
    process.stdout.write(`${JSON.stringify(payload, null, 2)}\n`);
    process.exitCode = 1;
  }
}

/**
 * 读取 CLI 输入任务。
 * @returns {Promise<object>}
 */
async function readTaskInput() {
  const args = process.argv.slice(2);
  const jsonIndex = args.indexOf("--json");
  if (jsonIndex >= 0 && args[jsonIndex + 1]) {
    return JSON.parse(args[jsonIndex + 1]);
  }

  const fileIndex = args.indexOf("--file");
  if (fileIndex >= 0 && args[fileIndex + 1]) {
    const fs = await import("node:fs/promises");
    const content = await fs.readFile(args[fileIndex + 1], "utf8");
    return JSON.parse(content);
  }

  const stdin = await readStdin();
  if (!stdin.trim()) {
    throw new Error("缺少输入，请使用 --json、--file 或标准输入提供任务 JSON");
  }
  return JSON.parse(stdin);
}

/**
 * 读取标准输入内容。
 * @returns {Promise<string>}
 */
async function readStdin() {
  const chunks = [];
  for await (const chunk of process.stdin) {
    chunks.push(Buffer.from(chunk));
  }
  return Buffer.concat(chunks).toString("utf8");
}

await main();
