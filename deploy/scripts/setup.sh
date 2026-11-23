#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

mkdir -p certs acme mail

if [ ! -f .env ]; then
	cp .env.example .env
	echo "Created .env. Set AUTH_DOMAIN, ACCOUNT_DOMAIN, POSTGRES_PASSWORD, and SERVER_NAME."
else
	echo ".env already exists. Review AUTH_DOMAIN, ACCOUNT_DOMAIN, POSTGRES_PASSWORD, and SERVER_NAME."
fi

if [ ! -f certs/fullchain.pem ] || [ ! -f certs/privkey.pem ]; then
	echo "TLS certificate missing. Place fullchain.pem and privkey.pem in:"
	echo "  $(pwd)/certs"
	echo "The certificate must cover AUTH_DOMAIN and ACCOUNT_DOMAIN."
	echo "If you enable the mail profile, it must also cover MAIL_HOSTNAME."
fi

echo
echo "Next steps:"
echo "  1. Edit .env"
echo "  2. Put the TLS certificate and key in ./certs"
echo "  3. docker compose up -d --build"
echo "  4. Register the first account at https://\$AUTH_DOMAIN/register"
echo
echo "Optional self-hosted mail:"
echo "  5. Set MAIL_* and SMTP_* in .env"
echo "  6. ./scripts/provision-mail.sh"
echo "  7. docker compose --profile mail up -d"
