#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

fail() {
	echo "error: $*" >&2
	exit 1
}

read_env() {
	key="$1"
	if [ ! -f .env ]; then
		return 0
	fi
	grep -E "^${key}=" .env 2>/dev/null | tail -n 1 | cut -d= -f2- || true
}

[ -f .env ] || fail "create .env from .env.example first"
command -v docker >/dev/null 2>&1 || fail "docker is required"
command -v python3 >/dev/null 2>&1 || fail "python3 is required to prepare Stalwart JSON payloads"
command -v openssl >/dev/null 2>&1 || fail "openssl is required"

MAIL_DOMAIN=$(read_env MAIL_DOMAIN)
MAIL_HOSTNAME=$(read_env MAIL_HOSTNAME)
MAIL_ADMIN_PASSWORD=$(read_env MAIL_ADMIN_PASSWORD)
MAIL_NO_REPLY_ADDRESS=$(read_env MAIL_NO_REPLY_ADDRESS)
MAIL_NO_REPLY_PASSWORD=$(read_env MAIL_NO_REPLY_PASSWORD)
SMTP_USERNAME=$(read_env SMTP_USERNAME)
SMTP_PASSWORD=$(read_env SMTP_PASSWORD)

[ -n "$MAIL_DOMAIN" ] || fail "set MAIL_DOMAIN in .env"
[ -n "$MAIL_HOSTNAME" ] || fail "set MAIL_HOSTNAME in .env"
[ -n "$MAIL_ADMIN_PASSWORD" ] || fail "set MAIL_ADMIN_PASSWORD in .env"
[ -n "$MAIL_NO_REPLY_ADDRESS" ] || fail "set MAIL_NO_REPLY_ADDRESS in .env"
[ -n "$MAIL_NO_REPLY_PASSWORD" ] || fail "set MAIL_NO_REPLY_PASSWORD in .env"

case "$MAIL_ADMIN_PASSWORD" in
	change-this*|"") fail "replace the MAIL_ADMIN_PASSWORD placeholder with a strong password" ;;
	*:*) fail "MAIL_ADMIN_PASSWORD must not contain ':'" ;;
esac
case "$MAIL_NO_REPLY_PASSWORD" in
	change-this*|"") fail "replace the MAIL_NO_REPLY_PASSWORD placeholder with a strong password" ;;
esac

MAIL_NO_REPLY_LOCAL=${MAIL_NO_REPLY_ADDRESS%@*}
[ "$MAIL_NO_REPLY_LOCAL" != "$MAIL_NO_REPLY_ADDRESS" ] ||
	fail "MAIL_NO_REPLY_ADDRESS must be a full address such as no-reply@${MAIL_DOMAIN}"

if [ -n "$SMTP_USERNAME" ] && [ "$SMTP_USERNAME" != "$MAIL_NO_REPLY_ADDRESS" ]; then
	fail "SMTP_USERNAME must equal MAIL_NO_REPLY_ADDRESS when using Stalwart"
fi
if [ -n "$SMTP_PASSWORD" ] && [ "$SMTP_PASSWORD" != "$MAIL_NO_REPLY_PASSWORD" ]; then
	fail "SMTP_PASSWORD must equal MAIL_NO_REPLY_PASSWORD when using Stalwart"
fi

[ -f certs/fullchain.pem ] || fail "missing certs/fullchain.pem"
[ -f certs/privkey.pem ] || fail "missing certs/privkey.pem"
if ! openssl x509 -in certs/fullchain.pem -noout -checkhost "$MAIL_HOSTNAME" >/dev/null 2>&1; then
	echo "warning: certs/fullchain.pem does not cover ${MAIL_HOSTNAME}" >&2
fi

mkdir -p mail

compose_mail() {
	docker compose --profile mail "$@"
}

cli() {
	user="$1"
	password="$2"
	shift 2
	docker compose --profile mail --profile mail-tools run --rm -T \
		-e STALWART_USER="$user" \
		-e STALWART_PASSWORD="$password" \
		mail-cli "$@"
}

wait_for_mail() {
	attempt=0
	while [ "$attempt" -lt 45 ]; do
		if compose_mail exec -T mail curl -fsS -o /dev/null \
			http://127.0.0.1:8080/healthz/live 2>/dev/null; then
			return 0
		fi
		attempt=$((attempt + 1))
		sleep 2
	done
	fail "Stalwart did not become healthy"
}

if [ -f mail/admin-credentials.txt ]; then
	echo "Using existing Stalwart administrator credentials."
	ADMIN_USER=$(sed -n 's/^username=//p' mail/admin-credentials.txt)
	ADMIN_PASSWORD=$(sed -n 's/^password=//p' mail/admin-credentials.txt)
	[ -n "$ADMIN_USER" ] && [ -n "$ADMIN_PASSWORD" ] ||
		fail "mail/admin-credentials.txt is incomplete"
	compose_mail up -d mail
	wait_for_mail
else
	echo "Starting Stalwart in bootstrap mode..."
	STALWART_RECOVERY_ADMIN="admin:${MAIL_ADMIN_PASSWORD}" compose_mail up -d mail
	wait_for_mail

	echo "Completing Stalwart setup for ${MAIL_DOMAIN} (${MAIL_HOSTNAME})..."
	setup_output=$(cli admin "$MAIL_ADMIN_PASSWORD" update Bootstrap \
		--field "defaultHostname=${MAIL_HOSTNAME}" \
		--field "defaultDomain=${MAIL_DOMAIN}" \
		--field requestTlsCertificate=false \
		--field generateDkimKeys=true)
	ADMIN_USER=$(printf '%s\n' "$setup_output" | sed -n 's/.*username: "\(.*\)"/\1/p' | head -n 1)
	ADMIN_PASSWORD=$(printf '%s\n' "$setup_output" | sed -n 's/.*secret: "\(.*\)"/\1/p' | head -n 1)
	[ -n "$ADMIN_USER" ] && [ -n "$ADMIN_PASSWORD" ] ||
		fail "could not parse the generated administrator credentials"
	umask 077
	printf 'username=%s\npassword=%s\n' "$ADMIN_USER" "$ADMIN_PASSWORD" >mail/admin-credentials.txt
	chmod 600 mail/admin-credentials.txt
	compose_mail restart mail
	wait_for_mail
fi

cli "$ADMIN_USER" "$ADMIN_PASSWORD" query Domain --fields id,name >/tmp/umc-auth-domains.txt ||
	fail "could not query Stalwart domains; check mail/admin-credentials.txt"
DOMAIN_ID=$(awk -v domain="$MAIL_DOMAIN" '$2 == domain {print $1; exit}' /tmp/umc-auth-domains.txt)
[ -n "$DOMAIN_ID" ] || fail "domain ${MAIL_DOMAIN} was not found in Stalwart"

python3 - "$MAIL_NO_REPLY_LOCAL" "$DOMAIN_ID" "$MAIL_NO_REPLY_PASSWORD" "$PWD/mail/account.json" <<'PY'
import json
import sys

name, domain_id, password, path = sys.argv[1:]
with open(path, "w", encoding="utf-8") as handle:
    json.dump({
        "name": name,
        "domainId": domain_id,
        "credentials": {"0": {"@type": "Password", "secret": password}},
    }, handle)
PY
chmod 600 mail/account.json
cli "$ADMIN_USER" "$ADMIN_PASSWORD" query Account --fields id,name,emailAddress \
	>/tmp/umc-auth-accounts.txt || fail "could not query Stalwart accounts"
NO_REPLY_ID=$(awk -v name="$MAIL_NO_REPLY_LOCAL" '$2 == name {print $1; exit}' /tmp/umc-auth-accounts.txt)
if [ -z "$NO_REPLY_ID" ]; then
	echo "Creating ${MAIL_NO_REPLY_ADDRESS}..."
	cli "$ADMIN_USER" "$ADMIN_PASSWORD" create Account/User --file /work/account.json ||
		fail "could not create ${MAIL_NO_REPLY_ADDRESS}"
else
	echo "Updating credentials for ${MAIL_NO_REPLY_ADDRESS}..."
	cli "$ADMIN_USER" "$ADMIN_PASSWORD" update Account "$NO_REPLY_ID" --file /work/account.json ||
		fail "could not update ${MAIL_NO_REPLY_ADDRESS}"
fi
rm -f mail/account.json

python3 - "$PWD/certs/fullchain.pem" "$PWD/certs/privkey.pem" "$PWD/mail/certificate.json" <<'PY'
import json
import sys

certificate_path, key_path, output_path = sys.argv[1:]
with open(certificate_path, encoding="utf-8") as handle:
    certificate = handle.read()
with open(key_path, encoding="utf-8") as handle:
    private_key = handle.read()
with open(output_path, "w", encoding="utf-8") as handle:
    json.dump({
        "certificate": {"@type": "Text", "value": certificate},
        "privateKey": {"@type": "Text", "secret": private_key},
    }, handle)
PY
chmod 600 mail/certificate.json
cli "$ADMIN_USER" "$ADMIN_PASSWORD" query Certificate --fields id,subjectAlternativeNames \
	>/tmp/umc-auth-certificates.txt || fail "could not query Stalwart certificates"
CERT_ID=$(awk -v host="$MAIL_HOSTNAME" 'NR > 1 && index($0, host) {print $1; exit}' \
	/tmp/umc-auth-certificates.txt)
if [ -z "$CERT_ID" ]; then
	cert_output=$(cli "$ADMIN_USER" "$ADMIN_PASSWORD" create Certificate --file /work/certificate.json) ||
		fail "could not create the Stalwart TLS certificate"
	CERT_ID=$(printf '%s\n' "$cert_output" | sed -n 's/^Created Certificate //p' | head -n 1)
else
	cli "$ADMIN_USER" "$ADMIN_PASSWORD" update Certificate "$CERT_ID" --file /work/certificate.json ||
		fail "could not update the Stalwart TLS certificate"
fi
rm -f mail/certificate.json
[ -n "$CERT_ID" ] || fail "could not determine the Stalwart certificate id"
cli "$ADMIN_USER" "$ADMIN_PASSWORD" update SystemSettings \
	--field "defaultCertificateId=${CERT_ID}" >/dev/null || fail "could not set the default certificate"

if grep -q '^STALWART_RECOVERY_ADMIN=' .env 2>/dev/null; then
	echo "warning: remove STALWART_RECOVERY_ADMIN from .env after provisioning" >&2
fi
echo "Applying TLS certificate and removing the bootstrap administrator..."
compose_mail up -d --force-recreate mail
wait_for_mail

cli "$ADMIN_USER" "$ADMIN_PASSWORD" get Domain "$DOMAIN_ID" >mail/domain.txt ||
	fail "could not read the Stalwart DNS zone"
sed -n '/DNS Zone File:/,$p' mail/domain.txt | sed '1d;s/^    //' >mail/dns-records.txt

if echo | openssl s_client -connect "127.0.0.1:465" -servername "$MAIL_HOSTNAME" \
	-verify_return_error >/dev/null 2>&1; then
	echo "TLS certificate verified for ${MAIL_HOSTNAME}"
else
	echo "warning: TLS verification failed; install a trusted certificate for ${MAIL_HOSTNAME}" >&2
fi

cat <<EOF

Mail provisioning complete.

Administrator UI (SSH tunnel required):
  ssh -L 8443:127.0.0.1:8443 <user>@<server>
  open https://127.0.0.1:8443/admin

Setup or recovery console:
  ssh -L 8080:127.0.0.1:8080 <user>@<server>
  open http://127.0.0.1:8080/admin

Administrator credentials:
  mail/admin-credentials.txt (mode 600)

SMTP for UMC Auth:
  SMTP_HOST=${MAIL_HOSTNAME}
  SMTP_PORT=465
  SMTP_TLS=implicit
  SMTP_USERNAME=${MAIL_NO_REPLY_ADDRESS}
  SMTP_PASSWORD=<MAIL_NO_REPLY_PASSWORD>

DNS records:
  mail/dns-records.txt

Next:
  1. Publish the DNS records, including MX, SPF, DKIM, DMARC, and PTR.
  2. Confirm that outbound TCP 25 is not blocked by the provider.
  3. Set EMAIL_VERIFICATION=required in .env.
  4. Run: docker compose up -d auth-api
EOF
