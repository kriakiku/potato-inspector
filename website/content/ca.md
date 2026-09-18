---
title: CA
weight: 40
---

Root CA lives at:

- `/data/ca/potatonetwork-ca.pem`
- `/data/ca/potatonetwork-ca-key.pem`

Created on first start if missing. Download: `GET /v1/ca.pem`.

Sidecars that use HTTPS through MITM must trust this CA (OS trust store, `SSL_CERT_FILE`, `NODE_EXTRA_CA_CERTS`, etc.).
