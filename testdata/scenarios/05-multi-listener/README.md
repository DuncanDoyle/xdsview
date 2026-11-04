# Scenario 05: Multiple Listeners

Gateway with an HTTP listener on 80 and an HTTPS listener on 443, both for `api.example.com`. One HTTPRoute with no `sectionName` attaches to both.

## What to observe

- One Envoy route per listener: `listener~80~api_example_com-...` and `https-api~api_example_com-...`
- Filtering by the HTTPRoute keeps both listeners

## K8S resources

- `k8s/gateway.yaml`: HTTP and HTTPS listeners
- `k8s/hook-tls-secrets.sh`: creates the self-signed `api-example-com` secret
- `k8s/httpbin.yaml`, `k8s/httproute-kgateway-system_service-httpbin-rg.yaml`: as in 02_7
- `k8s/api-example-com-httproute.yaml`: one rule to `httpbin`

## Envoy dump

`envoy/kgateway-<version>/config_dump.json`, with `server_info.json`.

## How to collect

```bash
hack/record-dumps.sh v2.5.0 05-multi-listener
```
