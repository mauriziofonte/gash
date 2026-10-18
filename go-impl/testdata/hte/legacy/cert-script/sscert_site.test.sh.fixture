#!/usr/bin/env bash

# Check that we're running as root
if [ "$(id -u)" != "0" ]; then
    echo >&2 "ERROR: This script must be run as root. Aborting."
    exit 1
fi

# Check for required commands
command -v openssl >/dev/null 2>&1 || { echo >&2 "ERROR: openssl binary is required but it's not installed. Aborting."; exit 1; }

# Generate a passphrase
export PASSPHRASE=$(head -c 500 /dev/urandom | tr -dc a-z0-9A-Z | head -c 128; echo)

# Runtime-compiled configuration file in a temporary location
CONFIG_FILE=$(mktemp /tmp/openssl.cnf.XXXXXX)

cat > $CONFIG_FILE <<-EOF
[req]
default_bits = 2048
prompt = no
default_md = sha256
x509_extensions = v3_req
distinguished_name = dn

[dn]
C = IT
ST = Italy
L = Italy
O = Acme Corp
OU = The Dev Team
emailAddress = webmaster@site.test
CN = site.test

[v3_req]
subjectAltName = @alt_names

[alt_names]
DNS.1 = *.site.test
DNS.2 = site.test
EOF

# Remove previous keys
echo " > Removing existing previous self-signed certs with pattern site.test.*"
if ls /etc/apache2/certs-selfsigned/site.test.* 1> /dev/null 2>&1; then
    rm -rf /etc/apache2/certs-selfsigned/site.test.*
fi

echo " > Generating certs for site.test"

# Generate our Private Key, CSR and Certificate
# Use SHA-2 as SHA-1 is unsupported from Jan 1, 2017
echo " > Generating RSA private key, 2048 bit long modulus"
openssl req -new -x509 -newkey rsa:2048 -sha256 -nodes -keyout "/etc/apache2/certs-selfsigned/site.test.key" -days 10950 -out "/etc/apache2/certs-selfsigned/site.test.crt" -passin pass:$PASSPHRASE -config "$CONFIG_FILE" 2>/dev/null >/dev/null

# OPTIONAL - write an info to see the details of the generated crt
echo " > Writing info to /etc/apache2/certs-selfsigned/site.test.info"
openssl x509 -noout -fingerprint -text < "/etc/apache2/certs-selfsigned/site.test.crt" > "/etc/apache2/certs-selfsigned/site.test.info"

# Protect the key
echo " > Protecting the key with chmod 400 /etc/apache2/certs-selfsigned/site.test.key"
chmod 400 "/etc/apache2/certs-selfsigned/site.test.key"

# Remove the config file
echo " > Removing the temporary config file $CONFIG_FILE"
rm -f $CONFIG_FILE

# Unset the passphrase
unset PASSPHRASE

# exit with success
exit 0
