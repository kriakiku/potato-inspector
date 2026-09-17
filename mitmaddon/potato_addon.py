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
            "extraDelayMs": 180,
            "rules": [],
            "bypassSni": [],
            "eventsURL": "http://127.0.0.1:9477/event",
            "capture": True,
        }
        self._compiled = []
        self._bypass = []

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
                    }
                )
            except re.error as e:
                ctx.log.warn(f"bad rule regex: {e}")
        self._bypass = [s.lower() for s in (self.config.get("bypassSni") or [])]

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
        if sni and sni.lower() in self._bypass:
            data.ignore_connection = True
            self._emit(
                "tls",
                f"TLS bypass SNI={sni}",
                {"sni": sni, "bypassed": True, "ok": True},
            )
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
        host = flow.request.host or ""
        path = flow.request.path or "/"
        delay = 0
        global_delay = int(self.config.get("extraDelayMs") or 180)
        for rule in self._compiled:
            if rule["host"].search(host) and rule["path"].search(path):
                delay = rule["delay"] if rule["delay"] > 0 else global_delay
                break
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
        if not self.config.get("capture", True):
            return
        req = flow.request
        resp = flow.response

        # Headers always — capture before any body work (body size must not skip them).
        req_headers = _headers_to_dict(req.headers)
        resp_headers = _headers_to_dict(resp.headers) if resp is not None else {}

        ct = ""
        if resp is not None:
            ct = resp.headers.get("content-type", "") or ""
        body_preview = ""
        stubbed = False
        body_bytes = 0
        if resp is not None:
            raw = resp.raw_content if resp.raw_content is not None else b""
            body_bytes = len(raw)
            if ct.startswith(("image/", "audio/", "video/", "application/octet-stream", "application/pdf", "application/zip", "font/", "application/wasm")):
                body_preview = f"[binary omitted: {ct}; {body_bytes} bytes]"
                stubbed = True
            else:
                # Never decode the full body — cap raw bytes first, then decode preview only.
                chunk = raw[:65536]
                charset = "utf-8"
                try:
                    charset = resp.headers.get_charset() or "utf-8"
                except Exception:
                    pass
                try:
                    body_preview = chunk.decode(charset, errors="replace")
                except Exception:
                    body_preview = chunk.decode("utf-8", errors="replace")
                if body_bytes > 65536:
                    body_preview += "\n…[truncated]"
                    stubbed = True

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
