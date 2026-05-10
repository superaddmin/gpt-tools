import json
import re
import time
import random
import secrets
import hashlib
import base64
import argparse
from datetime import datetime
from dataclasses import dataclass
from typing import Any, Dict, Optional
import urllib.parse
import urllib.request
import urllib.error

from curl_cffi import requests

# 引入 LuckMail SDK
from luckmail import LuckMailClient

# ==========================================
# LuckMail 邮箱服务配置
# ==========================================

# LuckMail 平台地址
LUCKMAIL_BASE_URL = "https://mails.luckyous.com/"
# LuckMail API Key（在平台「个人设置」页面生成）
LUCKMAIL_API_KEY = "ak_112233"
# 接码项目编码（在 LuckMail 平台创建的 OpenAI 项目）
LUCKMAIL_PROJECT_CODE = "openai"
# 邮箱类型（可选：ms_graph / ms_imap / self_built / google_variant）
LUCKMAIL_EMAIL_TYPE = "ms_graph"

# 初始化 LuckMail 客户端
luckmail_client = LuckMailClient(
    base_url=LUCKMAIL_BASE_URL,
    api_key=LUCKMAIL_API_KEY,
)


# ==========================================
# OAuth 授权与辅助函数
# ==========================================

AUTH_URL = "https://auth.openai.com/oauth/authorize"
TOKEN_URL = "https://auth.openai.com/oauth/token"
CLIENT_ID = "app_EMoamEEZ73f0CkXaXp7hrann"

DEFAULT_REDIRECT_URI = f"http://localhost:1455/auth/callback"
DEFAULT_SCOPE = "openid email profile offline_access"


def _b64url_no_pad(raw: bytes) -> str:
    return base64.urlsafe_b64encode(raw).decode("ascii").rstrip("=")


def _sha256_b64url_no_pad(s: str) -> str:
    return _b64url_no_pad(hashlib.sha256(s.encode("ascii")).digest())


def _random_state(nbytes: int = 16) -> str:
    return secrets.token_urlsafe(nbytes)


def _pkce_verifier() -> str:
    return secrets.token_urlsafe(64)


def _parse_callback_url(callback_url: str) -> Dict[str, Any]:
    """解析 OAuth 回调 URL，提取 code、state 等参数"""
    candidate = callback_url.strip()
    if not candidate:
        return {"code": "", "state": "", "error": "", "error_description": ""}

    if "://" not in candidate:
        if candidate.startswith("?"):
            candidate = f"http://localhost{candidate}"
        elif any(ch in candidate for ch in "/?#") or ":" in candidate:
            candidate = f"http://{candidate}"
        elif "=" in candidate:
            candidate = f"http://localhost/?{candidate}"

    parsed = urllib.parse.urlparse(candidate)
    query = urllib.parse.parse_qs(parsed.query, keep_blank_values=True)
    fragment = urllib.parse.parse_qs(parsed.fragment, keep_blank_values=True)

    for key, values in fragment.items():
        if key not in query or not query[key] or not (query[key][0] or "").strip():
            query[key] = values

    def get1(k: str) -> str:
        v = query.get(k, [""])
        return (v[0] or "").strip()

    code = get1("code")
    state = get1("state")
    error = get1("error")
    error_description = get1("error_description")

    if code and not state and "#" in code:
        code, state = code.split("#", 1)

    if not error and error_description:
        error, error_description = error_description, ""

    return {
        "code": code,
        "state": state,
        "error": error,
        "error_description": error_description,
    }


def _jwt_claims_no_verify(id_token: str) -> Dict[str, Any]:
    """解析 JWT Token 的 payload 部分（不验证签名）"""
    if not id_token or id_token.count(".") < 2:
        return {}
    payload_b64 = id_token.split(".")[1]
    pad = "=" * ((4 - (len(payload_b64) % 4)) % 4)
    try:
        payload = base64.urlsafe_b64decode((payload_b64 + pad).encode("ascii"))
        return json.loads(payload.decode("utf-8"))
    except Exception:
        return {}


def _decode_jwt_segment(seg: str) -> Dict[str, Any]:
    """解码 JWT 的某一段（header 或 payload）"""
    raw = (seg or "").strip()
    if not raw:
        return {}
    pad = "=" * ((4 - (len(raw) % 4)) % 4)
    try:
        decoded = base64.urlsafe_b64decode((raw + pad).encode("ascii"))
        return json.loads(decoded.decode("utf-8"))
    except Exception:
        return {}


def _to_int(v: Any) -> int:
    """安全转换为整数"""
    try:
        return int(v)
    except (TypeError, ValueError):
        return 0


def _post_form(url: str, data: Dict[str, str], timeout: int = 30) -> Dict[str, Any]:
    """发送 POST 表单请求"""
    body = urllib.parse.urlencode(data).encode("utf-8")
    req = urllib.request.Request(
        url,
        data=body,
        method="POST",
        headers={
            "Content-Type": "application/x-www-form-urlencoded",
            "Accept": "application/json",
        },
    )
    try:
        with urllib.request.urlopen(req, timeout=timeout) as resp:
            raw = resp.read()
            if resp.status != 200:
                raise RuntimeError(
                    f"Token 交换失败: {resp.status}: {raw.decode('utf-8', 'replace')}"
                )
            return json.loads(raw.decode("utf-8"))
    except urllib.error.HTTPError as exc:
        raw = exc.read()
        raise RuntimeError(
            f"Token 交换失败: {exc.code}: {raw.decode('utf-8', 'replace')}"
        ) from exc


def generate_password() -> str:
    """生成随机密码（至少8位，包含大小写和数字）"""
    return secrets.token_urlsafe(16)[:16] + "A1"


@dataclass(frozen=True)
class OAuthStart:
    """OAuth 授权启动参数"""
    auth_url: str
    state: str
    code_verifier: str
    redirect_uri: str


def generate_oauth_url(
    *, redirect_uri: str = DEFAULT_REDIRECT_URI, scope: str = DEFAULT_SCOPE
) -> OAuthStart:
    """生成 OAuth 授权 URL（含 PKCE）"""
    state = _random_state()
    code_verifier = _pkce_verifier()
    code_challenge = _sha256_b64url_no_pad(code_verifier)

    params = {
        "client_id": CLIENT_ID,
        "response_type": "code",
        "redirect_uri": redirect_uri,
        "scope": scope,
        "state": state,
        "code_challenge": code_challenge,
        "code_challenge_method": "S256",
        "prompt": "login",
        "id_token_add_organizations": "true",
        "codex_cli_simplified_flow": "true",
    }
    auth_url = f"{AUTH_URL}?{urllib.parse.urlencode(params)}"
    return OAuthStart(
        auth_url=auth_url,
        state=state,
        code_verifier=code_verifier,
        redirect_uri=redirect_uri,
    )


def submit_callback_url(
    *,
    callback_url: str,
    expected_state: str,
    code_verifier: str,
    redirect_uri: str = DEFAULT_REDIRECT_URI,
    account_email: str = "",
    account_password: str = "",
) -> str:
    """提交 OAuth 回调 URL，交换 Token 并返回配置 JSON"""
    cb = _parse_callback_url(callback_url)
    if cb["error"]:
        desc = cb["error_description"]
        raise RuntimeError(f"OAuth 错误: {cb['error']}: {desc}".strip())

    if not cb["code"]:
        raise ValueError("回调 URL 缺少 ?code= 参数")
    if not cb["state"]:
        raise ValueError("回调 URL 缺少 ?state= 参数")
    if cb["state"] != expected_state:
        raise ValueError("state 不匹配")

    token_resp = _post_form(
        TOKEN_URL,
        {
            "grant_type": "authorization_code",
            "client_id": CLIENT_ID,
            "code": cb["code"],
            "redirect_uri": redirect_uri,
            "code_verifier": code_verifier,
        },
    )

    access_token = (token_resp.get("access_token") or "").strip()
    refresh_token = (token_resp.get("refresh_token") or "").strip()
    id_token = (token_resp.get("id_token") or "").strip()
    expires_in = _to_int(token_resp.get("expires_in"))

    claims = _jwt_claims_no_verify(id_token)
    email = str(claims.get("email") or "").strip()
    auth_claims = claims.get("https://api.openai.com/auth") or {}
    account_id = str(auth_claims.get("chatgpt_account_id") or "").strip()

    now = int(time.time())
    expired_rfc3339 = time.strftime(
        "%Y-%m-%dT%H:%M:%SZ", time.gmtime(now + max(expires_in, 0))
    )
    now_rfc3339 = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(now))

    config = {
        "id_token": id_token,
        "access_token": access_token,
        "refresh_token": refresh_token,
        "account_id": account_id,
        "last_refresh": now_rfc3339,
        "email": email,
        "type": "codex",
        "expired": expired_rfc3339,
    }

    # 保存账号凭据（如果提供）
    if account_email:
        config["account_email"] = account_email
    if account_password:
        config["account_password"] = account_password

    return json.dumps(config, ensure_ascii=False, separators=(",", ":"))


# ==========================================
# 核心注册逻辑
# ==========================================


def run(proxy: Optional[str]) -> Optional[str]:
    proxies: Any = None
    if proxy:
        proxies = {"http": proxy, "https": proxy}

    s = requests.Session(proxies=proxies, impersonate="chrome")

    try:
        # 检查当前 IP 地区
        trace = s.get("https://cloudflare.com/cdn-cgi/trace", timeout=10)
        trace = trace.text
        loc_re = re.search(r"^loc=(.+)$", trace, re.MULTILINE)
        loc = loc_re.group(1) if loc_re else None
        print(f"[*] 当前 IP 地区: {loc}")
        if loc == "CN" or loc == "HK":
            raise RuntimeError("代理检查失败 - 不支持的地区")
    except Exception as e:
        print(f"[错误] 网络检查失败: {e}")
        return None

    # ==========================================
    # 第一步：通过 LuckMail SDK 创建接码订单，获取邮箱地址
    # ==========================================
    print("[*] 通过 LuckMail 创建接码订单...")
    try:
        order = luckmail_client.user.create_order(
            project_code=LUCKMAIL_PROJECT_CODE,
            email_type=LUCKMAIL_EMAIL_TYPE,
        )
        email = order.email_address
        order_no = order.order_no
        print(f"[*] 订单号: {order_no}")
        print(f"[*] 分配邮箱: {email}")
        print(f"[*] 超时时间: {order.expired_at}")
    except Exception as e:
        print(f"[错误] LuckMail 创建订单失败: {e}")
        return None

    password = generate_password()
    print(f"[*] 已生成密码: [已保存]")

    oauth = generate_oauth_url()
    url = oauth.auth_url

    try:
        resp = s.get(url, timeout=15)
        did = s.cookies.get("oai-did")
        print(f"[*] 设备 ID: {did}")

        # 注册请求体
        signup_body = f'{{"username":{{"value":"{email}","kind":"email"}},"screen_hint":"signup"}}'
        sen_req_body = f'{{"p":"","id":"{did}","flow":"authorize_continue"}}'

        # 获取 Sentinel Token
        sen_resp = requests.post(
            "https://sentinel.openai.com/backend-api/sentinel/req",
            headers={
                "origin": "https://sentinel.openai.com",
                "referer": "https://sentinel.openai.com/backend-api/sentinel/frame.html?sv=20260219f9f6",
                "content-type": "text/plain;charset=UTF-8",
            },
            data=sen_req_body,
            proxies=proxies,
            impersonate="chrome",
            timeout=15,
        )

        if sen_resp.status_code != 200:
            print(f"[错误] Sentinel 异常拦截，状态码: {sen_resp.status_code}")
            return None

        sen_token = sen_resp.json()["token"]
        sentinel = f'{{"p": "", "t": "", "c": "{sen_token}", "id": "{did}", "flow": "authorize_continue"}}'

        # 提交注册表单
        signup_resp = s.post(
            "https://auth.openai.com/api/accounts/authorize/continue",
            headers={
                "referer": "https://auth.openai.com/create-account",
                "accept": "application/json",
                "content-type": "application/json",
                "openai-sentinel-token": sentinel,
            },
            data=signup_body,
        )
        print(f"[*] 注册表单状态: {signup_resp.status_code}")

        # 检查响应
        try:
            signup_data = signup_resp.json()
            print(f"[*] 注册响应: {signup_data}")
        except:
            print(f"[*] 注册响应: {signup_resp.text[:300]}")

        # ==========================================
        # 第二步：提交密码（使用 /user/register 接口）
        # ==========================================
        print(f"[*] 提交密码...")

        register_body = json.dumps({"password": password, "username": email})
        print(f"[*] 生成的密码: {password[:4]}****")

        pwd_resp = s.post(
            "https://auth.openai.com/api/accounts/user/register",
            headers={
                "referer": "https://auth.openai.com/create-account/password",
                "accept": "application/json",
                "content-type": "application/json",
                "openai-sentinel-token": sentinel,
            },
            data=register_body,
            proxies=proxies,
        )
        print(f"[*] 密码提交状态: {pwd_resp.status_code}")

        if pwd_resp.status_code != 200:
            print(f"[!] 密码响应: {pwd_resp.text[:500]}")
            return None

        # 从注册响应中获取 continue_url
        try:
            register_json = pwd_resp.json()
            register_continue = register_json.get("continue_url", "")
            print(f"[*] 注册 continue_url: {register_continue}")
        except:
            register_continue = ""
            print(f"[*] 注册响应: {pwd_resp.text[:300]}")

        # ==========================================
        # 第三步：发送 OTP 验证码
        # ==========================================
        # 使用注册响应中的 continue_url 或默认 OTP 端点
        otp_url = register_continue if register_continue else "https://auth.openai.com/api/accounts/email-otp/send"
        print(f"[*] 发送 OTP: {otp_url}")

        otp_resp = s.post(
            otp_url,
            headers={
                "referer": "https://auth.openai.com/create-account/password",
                "accept": "application/json",
                "content-type": "application/json",
                "openai-sentinel-token": sentinel,
            },
        )
        print(f"[*] OTP 发送状态: {otp_resp.status_code}")

        # 打印响应内容（调试用）
        if otp_resp.status_code != 200:
            print(f"[!] OTP 响应: {otp_resp.text[:500]}")

        # ==========================================
        # 第四步：通过 LuckMail SDK 轮询等待验证码
        # ==========================================
        print(f"[*] 通过 LuckMail SDK 等待验证码...")

        def on_poll(result):
            """每次轮询的回调"""
            print(f"  轮询中... 状态: {result.status}")

        code_result = luckmail_client.user.wait_for_code(
            order_no=order_no,
            timeout=300,
            interval=3.0,
            on_poll=on_poll,
        )

        code = ""
        if code_result.status == "success" and code_result.verification_code:
            code = code_result.verification_code
            print(f"[*] 收到验证码: {code}")
        else:
            print(f"[!] 首次轮询未获取到验证码，状态: {code_result.status}")

            # 如果首次没拿到，尝试重发 OTP 并重新轮询
            for retry in range(2):
                retry_num = retry + 1
                print(f"[*] 重发 OTP (尝试 {retry_num}/2)...")

                otp_resp = s.post(
                    "https://auth.openai.com/api/accounts/passwordless/send-otp",
                    headers={
                        "referer": "https://auth.openai.com/create-account/password",
                        "accept": "application/json",
                        "content-type": "application/json",
                    },
                )

                if otp_resp.status_code == 409:
                    print(f"[!] 会话已过期: {otp_resp.text[:200]}")
                    break

                if otp_resp.status_code != 200:
                    print(f"[!] OTP 重发失败: {otp_resp.text[:200]}")
                    continue

                print(f"[*] OTP 已重发，等待新验证码...")

                # 重新创建订单并等待（因为原订单可能已超时）
                try:
                    new_order = luckmail_client.user.create_order(
                        project_code=LUCKMAIL_PROJECT_CODE,
                        email_type=LUCKMAIL_EMAIL_TYPE,
                        specified_email=email,  # 指定同一个邮箱
                    )
                    new_code_result = luckmail_client.user.wait_for_code(
                        order_no=new_order.order_no,
                        timeout=120,
                        interval=3.0,
                    )
                    if new_code_result.status == "success" and new_code_result.verification_code:
                        code = new_code_result.verification_code
                        print(f"[*] 成功获取验证码: {code}")
                        break
                except Exception as e:
                    print(f"[!] 重试创建订单失败: {e}")
                    continue

        if not code:
            print("[!] 未能获取 OTP 验证码")
            return None

        # ==========================================
        # 第五步：提交验证码
        # ==========================================
        code_body = f'{{"code":"{code}"}}'
        code_resp = s.post(
            "https://auth.openai.com/api/accounts/email-otp/validate",
            headers={
                "referer": "https://auth.openai.com/email-verification",
                "accept": "application/json",
                "content-type": "application/json",
            },
            data=code_body,
        )
        print(f"[*] 验证码校验状态: {code_resp.status_code}")

        # ==========================================
        # 第六步：创建账户
        # ==========================================
        create_account_body = '{"name":"Neo","birthdate":"2000-02-20"}'
        create_account_resp = s.post(
            "https://auth.openai.com/api/accounts/create_account",
            headers={
                "referer": "https://auth.openai.com/about-you",
                "accept": "application/json",
                "content-type": "application/json",
            },
            data=create_account_body,
        )
        create_account_status = create_account_resp.status_code
        print(f"[*] 账户创建状态: {create_account_status}")

        if create_account_status != 200:
            print(create_account_resp.text)
            return None

        # ==========================================
        # 第七步：获取授权 Cookie 并选择 Workspace
        # ==========================================
        auth_cookie = s.cookies.get("oai-client-auth-session")
        if not auth_cookie:
            print("[错误] 未能获取到授权 Cookie")
            return None

        print(f"[*] 授权 Cookie: {auth_cookie[:100]}...")

        auth_json = _decode_jwt_segment(auth_cookie.split(".")[0])
        print(f"[*] 授权 JSON 字段: {list(auth_json.keys())}")

        workspaces = auth_json.get("workspaces") or []
        if not workspaces:
            print("[!] 授权 Cookie 里没有 workspace 信息")
            print(f"[*] 可用字段: {list(auth_json.keys())}")
            # 尝试其他可能的字段名
            alt_keys = ["workspace", "workspace_id", "organizations", "orgs"]
            for key in alt_keys:
                if key in auth_json:
                    print(f"[*] 找到替代字段 '{key}': {auth_json[key]}")
            return None
        workspace_id = str((workspaces[0] or {}).get("id") or "").strip()
        if not workspace_id:
            print("[错误] 无法解析 workspace_id")
            return None

        select_body = f'{{"workspace_id":"{workspace_id}"}}'
        select_resp = s.post(
            "https://auth.openai.com/api/accounts/workspace/select",
            headers={
                "referer": "https://auth.openai.com/sign-in-with-chatgpt/codex/consent",
                "content-type": "application/json",
            },
            data=select_body,
        )

        if select_resp.status_code != 200:
            print(f"[错误] 选择 workspace 失败，状态码: {select_resp.status_code}")
            print(select_resp.text)
            return None

        continue_url = str((select_resp.json() or {}).get("continue_url") or "").strip()
        if not continue_url:
            print("[错误] workspace/select 响应里缺少 continue_url")
            return None

        # ==========================================
        # 第八步：跟踪重定向链，捕获最终回调 URL
        # ==========================================
        current_url = continue_url
        for _ in range(6):
            final_resp = s.get(current_url, allow_redirects=False, timeout=15)
            location = final_resp.headers.get("Location") or ""

            if final_resp.status_code not in [301, 302, 303, 307, 308]:
                break
            if not location:
                break

            next_url = urllib.parse.urljoin(current_url, location)
            if "code=" in next_url and "state=" in next_url:
                return submit_callback_url(
                    callback_url=next_url,
                    code_verifier=oauth.code_verifier,
                    redirect_uri=oauth.redirect_uri,
                    expected_state=oauth.state,
                    account_email=email,
                    account_password=password,
                )
            current_url = next_url

        print("[错误] 未能在重定向链中捕获到最终回调 URL")
        return None

    except Exception as e:
        print(f"[错误] 运行时发生错误: {e}")
        return None


def main() -> None:
    parser = argparse.ArgumentParser(description="OpenAI 自动注册脚本（使用 LuckMail SDK 接码）")
    parser.add_argument(
        "--proxy", default=None, help="代理地址，如 http://127.0.0.1:7890"
    )
    parser.add_argument("--once", action="store_true", help="只运行一次")
    parser.add_argument("--sleep-min", type=int, default=5, help="循环模式最短等待秒数")
    parser.add_argument(
        "--sleep-max", type=int, default=30, help="循环模式最长等待秒数"
    )
    args = parser.parse_args()

    sleep_min = max(1, args.sleep_min)
    sleep_max = max(sleep_min, args.sleep_max)

    count = 0
    print("[信息] OpenAI 自动注册脚本已启动（使用 LuckMail SDK 接码）")

    while True:
        count += 1
        print(
            f"\n[{datetime.now().strftime('%H:%M:%S')}] >>> 开始第 {count} 次注册流程 <<<"
        )

        try:
            token_json = run(args.proxy)

            if token_json:
                try:
                    t_data = json.loads(token_json)
                    fname_email = t_data.get("email", "unknown").replace("@", "_")
                except Exception:
                    fname_email = "unknown"

                file_name = f"token_{fname_email}_{int(time.time())}.json"

                with open(file_name, "w", encoding="utf-8") as f:
                    f.write(token_json)

                print(f"[*] 成功! Token 已保存至: {file_name}")
            else:
                print("[-] 本次注册失败。")

        except Exception as e:
            print(f"[错误] 发生未捕获异常: {e}")

        if args.once:
            break

        wait_time = random.randint(sleep_min, sleep_max)
        print(f"[*] 休息 {wait_time} 秒...")
        time.sleep(wait_time)


if __name__ == "__main__":
    main()
