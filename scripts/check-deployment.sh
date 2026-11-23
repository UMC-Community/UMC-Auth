#!/usr/bin/env bash
set -euo pipefail

: "${AUTH_DOMAIN:?Set AUTH_DOMAIN, for example auth.example.com}"
AUTH_DOMAIN="${AUTH_DOMAIN#http://}"
AUTH_DOMAIN="${AUTH_DOMAIN#https://}"
AUTH_DOMAIN="${AUTH_DOMAIN%%/*}"
BASE="https://${AUTH_DOMAIN}"
ACCOUNT_DOMAIN="${ACCOUNT_DOMAIN:-}"
if [ -n "$ACCOUNT_DOMAIN" ]; then
	ACCOUNT_DOMAIN="${ACCOUNT_DOMAIN#http://}"
	ACCOUNT_DOMAIN="${ACCOUNT_DOMAIN#https://}"
	ACCOUNT_DOMAIN="${ACCOUNT_DOMAIN%%/*}"
fi

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

pass() {
	echo "ok: $*"
}

command -v curl >/dev/null || fail "curl is required"
command -v openssl >/dev/null || fail "openssl is required"

read -r http_code redirect_url < <(curl -sS -o /dev/null -w '%{http_code} %{redirect_url}' "http://${AUTH_DOMAIN}/")
case "$http_code" in
	301|302|307|308) ;;
	*) fail "HTTP did not redirect to HTTPS (status ${http_code})" ;;
esac
case "$redirect_url" in
	https://*) ;;
	*) fail "HTTP redirect does not point to HTTPS (${redirect_url})" ;;
esac
pass "HTTP redirects to HTTPS"

[ "$(curl -fsS "${BASE}/healthz")" = "ok" ] || fail "health check failed"
pass "health check"

echo | openssl s_client -connect "${AUTH_DOMAIN}:443" -servername "${AUTH_DOMAIN}" \
	-verify_return_error >/dev/null 2>&1 || fail "TLS certificate is not trusted"
pass "TLS certificate chain is trusted"

echo | openssl s_client -connect "${AUTH_DOMAIN}:443" -servername "${AUTH_DOMAIN}" 2>/dev/null \
	| openssl x509 -noout -checkend 604800 >/dev/null 2>&1 \
	|| fail "TLS certificate expires within 7 days"
pass "TLS certificate is valid for at least 7 more days"

headers="$(curl -fsSI "${BASE}/")"
echo "$headers" | grep -qi '^strict-transport-security:' || fail "HSTS header is missing"
pass "HSTS header"

content_type="$(curl -fsSI "${BASE}/api/yggdrasil" | tr -d '\r' | awk -F': ' 'tolower($1)=="content-type" {print $2}')"
case "$content_type" in
	application/json*) ;;
	*) fail "Yggdrasil metadata is not JSON (${content_type})" ;;
esac
pass "Yggdrasil metadata"

if [ -n "$ACCOUNT_DOMAIN" ]; then
	root_json="$(curl -fsS "${BASE}/")" || fail "auth root did not return JSON"
	echo "$root_json" | grep -q '"service":"UMC Auth"' ||
		fail "auth root did not return the service metadata"
	pass "auth root returns service JSON"

	page_headers="$(curl -sSI "${BASE}/login")"
	echo "$page_headers" | head -1 | grep -q ' 404' ||
		fail "auth host served a page instead of a JSON 404"
	echo "$page_headers" | grep -qi '^content-type: application/json' ||
		fail "auth host 404 is not JSON"
	pass "auth host returns JSON 404 for page routes"
fi

if timeout 3 bash -c "</dev/tcp/${AUTH_DOMAIN}/8080" 2>/dev/null; then
	fail "port 8080 is directly reachable; expose only Nginx"
fi
pass "auth port is not exposed"

if [ -n "$ACCOUNT_DOMAIN" ]; then
	ACCOUNT_BASE="https://${ACCOUNT_DOMAIN}"
	headers="$(curl -fsSI "${ACCOUNT_BASE}/")" || fail "account panel is not reachable"
	echo "$headers" | grep -qi '^content-security-policy:' ||
		fail "account panel is missing Content-Security-Policy"
	echo "$headers" | grep -qi '^strict-transport-security:' ||
		fail "account panel is missing HSTS"
	pass "account panel headers"

	curl -fsS "${ACCOUNT_BASE}/login" | grep -q '<div id="app">' ||
		fail "account login page did not return the Vue panel"
	pass "account login page"

	curl -fsS "${ACCOUNT_BASE}/api/v1/meta" | grep -q '"server_name"' ||
		fail "account panel API proxy did not return metadata"
	pass "account panel API proxy"

	deep_code="$(curl -sS -o /dev/null -w '%{http_code}' "${ACCOUNT_BASE}/admin/users")"
	[ "$deep_code" = "200" ] ||
		fail "SPA deep link did not return 200 (status ${deep_code})"
	pass "SPA deep link"
fi

echo "All deployment checks passed for ${BASE}"
