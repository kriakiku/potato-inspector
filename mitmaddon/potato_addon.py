"""PotatoInspector mitmproxy addon: path delay + in-memory event ingest (not mitmweb)."""

from __future__ import annotations

import json
import re
import time
import urllib.error
import urllib.request
import uuid
from pathlib import Path

from mitmproxy import ctx, http, tls


class PotatoAddon:
    def __init__(self):
        self.config_path = None
        self.config = {
            "forceDisableCache": False,
            "rules": [],
            "eventsURL": "http://127.0.0.1:9477/event",
            "capture": True,
        }
        self._compiled = []

    def load(self, loader):
        loader.add_option(
            name="potato_config",
            typespec=str,
            default="/data/mitm-runtime.json",
            help="PotatoInspector runtime config JSON",
        )

    def configure(self, updated):
        self.config_path = ctx.options.potato_config
        self.reload()

    def reload(self):
        path = self.config_path or "/data/mitm-runtime.json"
        try:
            with open(path, "r", encoding="utf-8") as f:
                self.config = json.load(f)
        except Exception as e:
            ctx.log.warn(f"potato config load failed: {e}")
        self._compiled = []
        for rule in self.config.get("rules") or []:
            if not rule.get("enabled", True):
                continue
            try:
                self._compiled.append(
                    {
                        "host": re.compile(rule.get("hostRegex") or ".*", re.I),
                        "path": re.compile(rule.get("pathRegex") or ".*"),
                        "delay": int(rule.get("extraDelayMs") or 0),
                        "dest": (rule.get("dest") or "cf").strip() or "cf",
                    }
                )
            except re.error as e:
                ctx.log.warn(f"bad rule regex: {e}")
        self._system_ignore = bool(self.config.get("systemIgnoreEnabled", True))
        self._system_domains = [
            str(d).lower().strip().rstrip(".")
            for d in (self.config.get("systemIgnoreDomains") or [])
            if d
        ]
        self._custom_domains = []
        for e in self.config.get("customIgnore") or []:
            if not isinstance(e, dict) or not e.get("enabled", False):
                continue
            d = str(e.get("domain") or "").lower().strip().rstrip(".")
            if d.startswith("*."):
                d = d[2:]
            if d:
                self._custom_domains.append(d)

    def _suffix_match(self, host: str, domains) -> bool:
        if not host or not domains:
            return False
        h = host.lower().strip().rstrip(".")
        for d in domains:
            if h == d or h.endswith("." + d):
                return True
        return False

    def _ignore_match(self, host: str) -> bool:
        if self._system_ignore and self._suffix_match(host, self._system_domains):
            return True
        return self._suffix_match(host, self._custom_domains)

    def _emit(self, typ: str, summary: str, detail: dict):
        try:
            if self.config_path and Path(self.config_path).exists():
                mtime = Path(self.config_path).stat().st_mtime
                if getattr(self, "_mtime", 0) != mtime:
                    self._mtime = mtime
                    self.reload()
        except Exception:
            pass
        if not self.config.get("capture", True):
            return
        ev = {
            "id": str(uuid.uuid4()),
            "type": typ,
            "ts": time.strftime("%Y-%m-%dT%H:%M:%S.", time.gmtime())
            + f"{int((time.time() % 1) * 1e6):06d}Z",
            "summary": summary,
            "detail": detail,
        }
        url = self.config.get("eventsURL") or "http://127.0.0.1:9477/event"
        data = json.dumps(ev, ensure_ascii=False).encode("utf-8")
        req = urllib.request.Request(
            url,
            data=data,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=1.0) as resp:
                _ = resp.read()
        except (urllib.error.URLError, TimeoutError, OSError) as e:
            ctx.log.debug(f"potato event ingest failed: {e}")

    def tls_clienthello(self, data: tls.ClientHelloData):
        sni = ""
        try:
            sni = data.client_hello.sni or ""
        except Exception:
            pass
        if not sni:
            return
        sni_l = sni.lower()
        if self._ignore_match(sni_l):
            data.ignore_connection = True
            return

    def tls_established_client(self, data: tls.TlsData):
        sni = ""
        alpn = ""
        version = ""
        cn = ""
        sans = []
        ok = True
        err = ""
        try:
            sni = getattr(data.conn, "sni", None) or ""
            if data.conn.tls_version:
                version = str(data.conn.tls_version)
            if data.conn.alpn:
                alpn = data.conn.alpn.decode() if isinstance(data.conn.alpn, bytes) else str(data.conn.alpn)
            cert = data.conn.certificate_list[0] if data.conn.certificate_list else None
            if cert is not None:
                try:
                    cn = cert.subject.rfc4514_string()
                except Exception:
                    cn = str(getattr(cert, "cn", "") or "")
                try:
                    sans = [str(x) for x in (cert.extensions.get_extension_for_oid(
                        __import__("cryptography.x509", fromlist=["oid"]).oid.ExtensionOID.SUBJECT_ALTERNATIVE_NAME
                    ).value.get_values_for_type(__import__("cryptography.x509", fromlist=["DNSName"]).DNSName))]
                except Exception:
                    sans = []
        except Exception as e:
            ok = False
            err = str(e)
        self._emit(
            "tls",
            f"TLS {sni} {version} alpn={alpn}",
            {
                "sni": sni,
                "version": version,
                "alpn": alpn,
                "ok": ok,
                "error": err,
                "leafCN": cn,
                "leafSAN": sans,
                "durationMs": 0,
            },
        )

    def request(self, flow: http.HTTPFlow):
        self.reload_soft()
        if self.config.get("forceDisableCache"):
            _force_disable_cache_request(flow.request)
        host = flow.request.host or ""
        path = flow.request.path or "/"
        delay = 0
        matched = None
        for rule in self._compiled:
            if rule["host"].search(host) and rule["path"].search(path):
                matched = rule
                break
        if matched is not None:
            if matched["delay"] > 0:
                delay = matched["delay"]
            else:
                delay = _path_extra_delay_ms(self.config, matched.get("dest") or "cf")
        if delay > 0:
            time.sleep(delay / 1000.0)
            flow.metadata["potato_extra_delay_ms"] = delay

    def reload_soft(self):
        try:
            if self.config_path and Path(self.config_path).exists():
                mtime = Path(self.config_path).stat().st_mtime
                if getattr(self, "_mtime", 0) != mtime:
                    self._mtime = mtime
                    self.reload()
        except Exception:
            pass

    def response(self, flow: http.HTTPFlow):
        self.reload_soft()
        if self.config.get("forceDisableCache") and flow.response is not None:
            _force_disable_cache_response(flow.response)
        if not self.config.get("capture", True):
            return
        req = flow.request
        resp = flow.response

        # Headers always — capture before any body work (body size must not skip them).
        req_headers = _headers_to_dict(req.headers)
        resp_headers = _headers_to_dict(resp.headers) if resp is not None else {}

        req_ct = req.headers.get("content-type", "") or ""
        req_preview, req_stubbed, req_bytes = _body_preview(req, req_ct)

        ct = ""
        body_preview = ""
        stubbed = False
        body_bytes = 0
        if resp is not None:
            ct = resp.headers.get("content-type", "") or ""
            body_preview, stubbed, body_bytes = _body_preview(resp, ct)

        status = resp.status_code if resp else 0
        scheme = req.scheme or "https"
        host = req.pretty_host if hasattr(req, "pretty_host") else req.host
        path = req.path or "/"
        if req.port and req.port not in (80, 443):
            url = f"{scheme}://{host}:{req.port}{path}"
        else:
            url = f"{scheme}://{host}{path}"
        summary = f"{req.method} {url} → {status}"
        detail = {
            "method": req.method,
            "url": url,
            "host": host,
            "path": path,
            "status": status,
            "requestHeaders": req_headers,
            "responseHeaders": resp_headers,
            "requestContentType": req_ct,
            "requestBodyPreview": req_preview,
            "requestBodyStubbed": req_stubbed,
            "requestBodyBytes": req_bytes,
            "bodyPreview": body_preview,
            "bodyStubbed": stubbed,
            "bodyBytes": body_bytes,
            "contentType": ct,
            "extraDelayMs": flow.metadata.get("potato_extra_delay_ms", 0),
            "timing": {
                "timestampStart": flow.request.timestamp_start,
                "timestampEnd": flow.response.timestamp_end if flow.response else None,
            },
        }
        self._emit("http", summary, detail)


_BODY_PREVIEW_MAX = 65536
_BINARY_CT_PREFIXES = (
    "image/",
    "audio/",
    "video/",
    "application/octet-stream",
    "application/pdf",
    "application/zip",
    "font/",
    "application/wasm",
)


def _body_preview(msg, content_type: str) -> tuple:
    """Return (preview_text, stubbed, byte_len) from a mitmproxy request/response."""
    if msg is None:
        return "", False, 0
    ct = (content_type or "").lower()
    ct_main = ct.split(";")[0].strip()
    try:
        raw = msg.content if msg.content is not None else b""
    except Exception:
        raw = msg.raw_content if getattr(msg, "raw_content", None) is not None else b""
    if raw is None:
        raw = b""
    body_bytes = len(raw)
    if body_bytes == 0:
        return "", False, 0

    ce = ""
    try:
        ce = (msg.headers.get("content-encoding", "") or "").lower()
    except Exception:
        pass

    if ct_main.startswith("multipart/"):
        return f"[multipart omitted; {body_bytes} bytes]", True, body_bytes
    if any(ct_main.startswith(p) for p in _BINARY_CT_PREFIXES):
        return f"[binary omitted: {ct_main or ct}; {body_bytes} bytes]", True, body_bytes

    chunk = raw[:_BODY_PREVIEW_MAX]
    charset = "utf-8"
    try:
        charset = msg.headers.get_charset() or "utf-8"
    except Exception:
        pass
    try:
        preview = chunk.decode(charset, errors="replace")
    except Exception:
        preview = chunk.decode("utf-8", errors="replace")
    if len(chunk) >= 2 and chunk[:2] == b"\x1f\x8b":
        return (
            f"[compressed body not decoded; content-encoding={ce or '?'}; {body_bytes} bytes]",
            True,
            body_bytes,
        )
    stubbed = False
    if body_bytes > _BODY_PREVIEW_MAX:
        preview += "\n…[truncated]"
        stubbed = True
    return preview, stubbed, body_bytes


_CONDITIONAL_REQ_HEADERS = (
    "If-None-Match",
    "If-Modified-Since",
    "If-Match",
    "If-Unmodified-Since",
    "If-Range",
)


def _path_extra_delay_ms(config: dict, dest: str) -> int:
    """One-way extra delay from destination RTT vs CF, minus host baseline delta."""
    if not dest or dest == "cf":
        return 0
    pd = config.get("pathDelay") or {}
    rtt = pd.get("rttToDest") or {}
    host = pd.get("hostRtt") or {}
    try:
        rtt_dest = int(rtt.get(dest) or 0)
        rtt_cf = int(rtt.get("cf") or 0)
    except (TypeError, ValueError):
        return 0
    path_delta = max(0, rtt_dest - rtt_cf)
    try:
        host_delta = max(0, int(host.get(dest) or 0) - int(host.get("cf") or 0))
    except (TypeError, ValueError):
        host_delta = 0
    extra_rtt = max(0, path_delta - host_delta)
    return extra_rtt // 2


_RESPONSE_VALIDATOR_HEADERS = (
    "ETag",
    "Expires",
    "Last-Modified",
)


def _del_headers(headers, names):
    for name in names:
        try:
            if name in headers:
                del headers[name]
        except Exception:
            pass


def _force_disable_cache_request(req: http.Request):
    _del_headers(req.headers, _CONDITIONAL_REQ_HEADERS)
    req.headers["Cache-Control"] = "no-cache"
    req.headers["Pragma"] = "no-cache"


def _force_disable_cache_response(resp: http.Response):
    _del_headers(resp.headers, _RESPONSE_VALIDATOR_HEADERS)
    resp.headers["Cache-Control"] = "no-store, no-cache, must-revalidate"


def _headers_to_dict(headers) -> dict:
    out = {}
    if headers is None:
        return out
    try:
        for k, v in headers.items(multi=True):
            key = str(k)
            val = str(v)
            if key in out:
                out[key] = f"{out[key]}, {val}"
            else:
                out[key] = val
        return out
    except TypeError:
        pass
    try:
        return {str(k): str(v) for k, v in dict(headers).items()}
    except Exception:
        return {}


addons = [PotatoAddon()]
