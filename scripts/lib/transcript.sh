#!/usr/bin/env bash
# The caller owns its EXIT trap and supplies the original command status.
start_transcript() {
    local pipe_directory
    pipe_directory="$(mktemp -d "${TMPDIR:-/tmp}/platform-transcript.XXXXXX")"
    mkfifo "$pipe_directory/output"
    exec 3>&1 4>&2
    # An explicit background child is waitable on macOS Bash 3.2 as well as
    # newer Bash versions; process-substitution children are not portable here.
    tee "$1" < "$pipe_directory/output" &
    transcript_pid=$!
    exec > "$pipe_directory/output" 2>&1
    # Both ends are open; the filesystem entry is no longer needed.
    rm "$pipe_directory/output"
    rmdir "$pipe_directory"
}

finish_transcript() {
    local status="$1" logger_status=0
    # Close both pipe writers before waiting, otherwise tee cannot reach EOF.
    exec 1>&3 2>&4 3>&- 4>&-
    wait "$transcript_pid" || logger_status=$?
    if [[ "$logger_status" != 0 ]]; then
        printf 'Transcript writer failed with status %s.\n' "$logger_status" >&2
        [[ "$status" != 0 ]] || status="$logger_status"
    fi
    return "$status"
}
