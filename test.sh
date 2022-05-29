#!/bin/bash

source .env || true

check-requirements() {
    silent type icbmUserDb || panic \
        "icbmUserDb needs to be a shell function or executable in the path." \
        "When invoked, it should print the JSON for the icbm user database."
    [ -n "$APITESTKEY" ] || panic "Please export the environment variable APITESTKEY=<icbm-fridge-test-key> and try again."
}

log() {
    printf >&2 "%s\n" "$@"
}

panic() {
    log "$@"
    exit 1
}

silent() {
    >/dev/null 2>&1 "$@"
}

api() {
    curl -sL --header "x-icbm-api-key: $APITESTKEY" "$@"
}

get() {
    url="$1"
    silent api -X GET "$url"
}

post() {
    url="$1"
    data="$2"
    api -X post "$url" -d@<(echo "$data")
}

payload() {
    case $1 in
        Lunarville-beta|Lunarville)
            printf '{"FridgeName":"%s", ' "$1" ;;
        *) return ;;
    esac

    cat << 'EOF'
    "RawMassFull": 800000,
    "RawMassTare": 300000,
    "RawSamples": [
        { "PubFillRatio": 0.48296, "RawFillRatio": 0.612222, "RawMass": 606111, "Timestamp": "2018-09-13T05:11:32Z" },
        { "PubFillRatio": 0.49112, "RawFillRatio": 0.618334, "RawMass": 609167, "Timestamp": "2018-09-13T05:11:33Z" },
        { "PubFillRatio": 0.49926, "RawFillRatio": 0.624444, "RawMass": 612222, "Timestamp": "2018-09-13T05:11:34Z" },
        { "PubFillRatio": 0.50741, "RawFillRatio": 0.630556, "RawMass": 615278, "Timestamp": "2018-09-13T05:11:35Z" },
        { "PubFillRatio": 0.51556, "RawFillRatio": 0.636666, "RawMass": 618333, "Timestamp": "2018-09-13T05:11:36Z" },
        { "PubFillRatio": 0.52370, "RawFillRatio": 0.642778, "RawMass": 621389, "Timestamp": "2018-09-13T05:11:37Z" }
    ],
    "StableSamples": [
        { "PubFillRatio": 0.51235, "RawFillRatio": 0.654312, "RawMass": 611117, "Timestamp": "2021-05-09T23:34:45Z" }
    ]
EOF
    printf '%s\n' '}'
}


ExitCode=0
try() {
    cmd="$1"
    url="$2"

    output=$("$@")
    case $? in
        0)
            info "$(printf "PASS  %-4s  %-50s\n" "$cmd" "$url")"
            ;;
        *)
            log "$(printf "FAIL  %-4s  %-50s %s\n" "$cmd" "$url" "$output ($?)")"
            ExitCode=2
            ;;
    esac
}

test-icbm() {
    info "$(api -X GET https://icbm.api.evq.io/version)"
    info "$(api -X GET https://lunarville.org/version)"

    try post https://lunarville.org/icbm/v1 "$(payload Lunarville-beta)"
    try post https://icbm.api.evq.io/icbm/v1 "$(payload Lunarville-beta)"
    try post https://api.evq.io:8081/icbm/v1 "$(payload Lunarville-beta)"

    try get https://icbm.api.evq.io/data/Lunarville.tsv
    try get https://api.evq.io:8081/data/Lunarville.tsv
    try get http://lunarville.org/
    try get https://lunarville.org/data/Lunarville.tsv

    exit $ExitCode
}

main() {
    check-requirements
    # Define a default "info" function which does nothing.
    info() { true; }
    case $1 in
        -v|--verbose)
            # Unless the user requests otherwise, in which case log informational messages.
            info() { log "$@"; }
            ;;
    esac
    test-icbm
}

main "$@"
