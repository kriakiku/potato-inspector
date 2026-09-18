---
title: OpenAPI
weight: 21
toc: false
sidebar:
  hide: true
---

Interactive API reference ([ReDoc](https://redocly.com/docs/redoc/)). Spec downloads:

- [swagger.json](https://kriakiku.github.io/potato-network/swagger.json)
- [swagger.yaml](https://kriakiku.github.io/potato-network/swagger.yaml)

See also the human [API](api) notes.

<div id="redoc-container" class="pn-redoc"></div>
<script src="https://cdn.redoc.ly/redoc/v2.1.5/bundles/redoc.standalone.js"></script>
<script>
  Redoc.init(
    "https://kriakiku.github.io/potato-network/swagger.json",
    {
      scrollYOffset: 64,
      hideDownloadButton: false,
      expandResponses: "200",
    },
    document.getElementById("redoc-container")
  );
</script>
