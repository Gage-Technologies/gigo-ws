#!/usr/bin/env sh
set -eux
# Sleep for a good long while before exiting.
# This is to allow folks to exec into a failed workspace and poke around to
# troubleshoot.
waitonexit() {
	echo "=== Agent script exited with non-zero code. Sleeping 24h to preserve logs..."
	sleep 86400
}
trap waitonexit EXIT
BINARY_DIR=$(mktemp -d -t gigo.XXXXXX)
BINARY_NAME=gigo
BINARY_URL=${ACCESS_URL}bin/gigo-darwin-${ARCH}
cd "$BINARY_DIR"
# Attempt to download the gigo agent.
# This could fail for a number of reasons, many of which are likely transient.
# So just keep trying!
while :; do
	curl -fsSL --compressed "${BINARY_URL}" -o "${BINARY_NAME}" && break
	status=$?
	echo "error: failed to download gigo agent using curl"
	echo "curl exit code: ${status}"
	echo "Trying again in 1 second..."
	sleep 1
done

if ! chmod +x $BINARY_NAME; then
	echo "Failed to make $BINARY_NAME executable"
	exit 1
fi

export GIGO_AGENT_URL="${ACCESS_URL}"
exec ./$BINARY_NAME agent
