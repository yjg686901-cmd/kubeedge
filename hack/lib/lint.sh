#!/usr/bin/env bash

###
#Copyright 2020 The KubeEdge Authors.
#
#Licensed under the Apache License, Version 2.0 (the "License");
#you may not use this file except in compliance with the License.
#You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
#Unless required by applicable law or agreed to in writing, software
#distributed under the License is distributed on an "AS IS" BASIS,
#WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
#See the License for the specific language governing permissions and
#limitations under the License.
###

set -o errexit
set -o nounset
set -o pipefail

function kubeedge::lint::init() {
    SED_CMD=""
    KUBEEDGE_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"

    if [[ "$OSTYPE" == "darwin"* ]]
    then
        if ! which gsed >/dev/null 2>&1
        then
            echo "Please install gnu-sed (brew install gnu-sed)"
            exit 1
        else
            SED_CMD=`which gsed`
        fi
    elif [[ "$OSTYPE" == "linux"* ]]
    then
        if ! which sed >/dev/null 2>&1
        then
            echo "Please install sed"
            exit 1
        else
            SED_CMD=`which sed`
        fi
    else
        echo "Unsupported OS $OSTYPE"
        exit 1
    fi
}

kubeedge::lint::check() {
    kubeedge::lint::init
    cd "${KUBEEDGE_ROOT}"

    echo "start lint ..."
    echo "check whitespace in working tree ..."

    # Read-only checks: do not rewrite repository files.
    git diff --check HEAD

    # Include committed PR changes when a base branch is available.
    if [[ -n "${GITHUB_BASE_REF:-}" ]]; then
        local base_ref="refs/remotes/origin/${GITHUB_BASE_REF}"
        local base_commit

        if ! git rev-parse --verify "${base_ref}^{commit}" >/dev/null 2>&1; then
            echo "Cannot resolve PR base ref: ${base_ref}" >&2
            return 1
        fi

        base_commit="$(git merge-base HEAD "${base_ref}")" || return 1
        git diff --check "${base_commit}" HEAD
    fi

    echo "check any issue by golangci-lint ..."
    GOOS="linux" golangci-lint run -v --timeout=12m

    echo "check any issue by golangci-lint under staging dir ..."
    cd "${KUBEEDGE_ROOT}/staging/src/github.com/kubeedge/beehive"
    GOOS="linux" golangci-lint run -v --timeout=1m

    cd "${KUBEEDGE_ROOT}/staging/src/github.com/kubeedge/mapper-framework"
    GOOS="linux" golangci-lint run -v --timeout=2m

    echo "check gofmt and goimports ..."
    cd "${KUBEEDGE_ROOT}"

    local fmt_output
    fmt_output="$(mktemp)" || return 1

    if ! GOOS="linux" golangci-lint fmt --diff >"${fmt_output}" 2>&1; then
        cat "${fmt_output}"
        rm -f "${fmt_output}"
        return 1
    fi

    if [[ -s "${fmt_output}" ]]; then
        cat "${fmt_output}"
        rm -f "${fmt_output}"
        echo "Go formatting differences detected" >&2
        return 1
    fi

    rm -f "${fmt_output}"
    echo "Go formatting check passed"
}
