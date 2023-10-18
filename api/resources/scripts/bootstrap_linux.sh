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
BINARY_NAME=gigo-agent
BINARY_URL=${ACCESS_URL}internal/v1/ws/agent
cd "$BINARY_DIR"
# Attempt to download the gigo agent.
# This could fail for a number of reasons, many of which are likely transient.
# So just keep trying!
while :; do
	# Try a number of different download tools, as we don not know what we
	# will have available.
	status=""
	if command -v curl >/dev/null 2>&1; then
		curl -fsSL -H "Gigo-Agent-Token: ${GIGO_AGENT_TOKEN}" -H "Gigo-Workspace-Id: ${GIGO_WORKSPACE_ID}" "${BINARY_URL}" -o "${BINARY_NAME}" && break
		status=$?
	elif command -v wget >/dev/null 2>&1; then
		wget -q --header="Gigo-Agent-Token: ${GIGO_AGENT_TOKEN}" --header="Gigo-Workspace-Id: ${GIGO_WORKSPACE_ID}" "${BINARY_URL}" -O "${BINARY_NAME}" && break
		status=$?
	elif command -v busybox >/dev/null 2>&1; then
		busybox wget -q --header="Gigo-Agent-Token: ${GIGO_AGENT_TOKEN}" --header="Gigo-Workspace-Id: ${GIGO_WORKSPACE_ID}" "${BINARY_URL}" -O "${BINARY_NAME}" && break
		status=$?
	else
		echo "error: no download tool found, please install curl, wget or busybox wget"
		exit 127
	fi
	echo "error: failed to download gigo agent"
	echo "       command returned: ${status}"
	echo "Trying again in 1 second..."
	sleep 1
done

if ! chmod +x $BINARY_NAME; then
	echo "Failed to make $BINARY_NAME executable"
	exit 1
fi

export GIGO_AGENT_URL="${ACCESS_URL}"
exec ./$BINARY_NAME
