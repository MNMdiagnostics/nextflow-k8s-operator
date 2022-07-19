#!/bin/bash

if [ -z "$1" ]; then
    SCMFILE="$HOME/.nextflow/scm"
else
    SCMFILE="$1"
fi

if [ ! -f "$SCMFILE" ]; then
    echo Make a YAML for a Kubernetes secret out of your Nextflow SCM file
    echo Usage:
    echo "  make_scm_secret.sh [scm_file]"
    echo If scm_file is not specified, ~/.nextflow/scm is used.
    exit 1
fi

cat <<EOF
apiVersion: v1
kind: Secret
type: Opaque
metadata:
  name: nf-scm-secret
data:
  scm: |
EOF

cat "$SCMFILE" | base64 | sed -e 's/^/    /'
