#!/bin/bash

# setup an example payload
read -r -d '' samples_body <<EOF
    "RawMassFull": 800000,
    "RawMassTare": 300000,
    "RawSamples": [
        {
            "PubFillRatio": 0.48296266666666665,
            "RawFillRatio": 0.612222,
            "RawMass": 606111,
            "Timestamp": "2018-09-13T05:11:32Z"
        },
        {
            "PubFillRatio": 0.491112,
            "RawFillRatio": 0.618334,
            "RawMass": 609167,
            "Timestamp": "2018-09-13T05:11:33Z"
        },
        {
            "PubFillRatio": 0.4992586666666667,
            "RawFillRatio": 0.624444,
            "RawMass": 612222,
            "Timestamp": "2018-09-13T05:11:34Z"
        },
        {
            "PubFillRatio": 0.507408,
            "RawFillRatio": 0.630556,
            "RawMass": 615278,
            "Timestamp": "2018-09-13T05:11:35Z"
        },
        {
            "PubFillRatio": 0.5155546666666667,
            "RawFillRatio": 0.636666,
            "RawMass": 618333,
            "Timestamp": "2018-09-13T05:11:36Z"
        },
        {
            "PubFillRatio": 0.523704,
            "RawFillRatio": 0.642778,
            "RawMass": 621389,
            "Timestamp": "2018-09-13T05:11:37Z"
        }
    ],
    "StableSamples": [
        {
            "PubFillRatio": 0.5123456789,
            "RawFillRatio": 0.6543,
            "RawMass": 611117,
            "Timestamp": "2021-05-09T23:34:45Z"
        }
    ]
EOF

# beta fridge
betaPayload='{"FridgeName":"Lunarville-beta", '
betaPayload+="$samples_body"
betaPayload+='}'

# live fridge
livePayload='{"FridgeName":"Lunarville", '
livePayload+="$samples_body"
livePayload+='}'

APITESTKEY=some-long-hash-here-generated-via-sha256sum

ExitCode=0

function Log {
    printf >&2 "$@"
}

function Silent {
    >/dev/null 2>&1 "$@"
}

function Api {
    curl -sfL --header "x-icbm-api-key: $APITESTKEY" "$@"
}

function Post {
    url="$1"
    payload="$2"
    Api -X POST "$url" -d@<(echo "$payload")
}

function Get {
    url="$1"
    Silent Api -X GET "$url"
}

function try() {
    cmd="$1"
    url="$2"
    payload="$3"

    output=$($cmd "${url}" "$payload")
    case $? in
        0)
            Log "PASS  %-4s  %-50s %s\n" "$cmd" "$url" "$output"
            ;;
        *)
            Log "FAIL  %-4s  %-50s %s\n" "$cmd" "$url" "$output"
            ExitCode=2
            ;;
    esac
}

Api -X GET https://icbm.api.evq.io/version

try Post https://icbm.api.evq.io:8081/icbm/v1 "$betaPayload"
try Post https://api.evq.io:8081/icbm/v1 "$betaPayload" 
try Post https://icbm.api.evq.io/icbm/v1 "$betaPayload" 

try Get https://icbm.api.evq.io/data/Lunarville.tsv
try Get https://api.evq.io:8081/data/Lunarville.tsv
try Get http://lunarville.org/

# try Post http://localhost/icbm/v1 "$betaPayload"  # will fail

exit $ExitCode
