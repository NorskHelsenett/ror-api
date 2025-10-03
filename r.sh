#!/usr/bin/bash

# 1.  Paths 
TEMPLATE="./hacks/vscode/launch.json"  
DEST="./.vscode/launch.json"           
ENV_FILE="./hacks/envs/env"             
DEBUG_SH="./debug.sh"

if [ ! -d ./.vscode ]; then
    mkdir -p ./.vscode;
fi

if [ ! -f "$DEST" ]; then
    set -euo pipefail

    # 2.  Build the JSON *object* for the env block
    ENV_JSON=$(awk -F= '
      /^[^#]/ {
        gsub(/"/, "\\\"", $2)          # escape double quotes
        gsub(/\\/, "\\\\\\\"", $2)     # escape backslashes
        printf "                \"%s\":\"%s\",\n", $1, $2
    }
    ' "$ENV_FILE" | sed '$s/,$//')      # drop the final comma    
  
    # Wrap it in braces
    ENV_JSON="{\\n$ENV_JSON\\n}"
    
    # 4.  Replace the placeholder in the template
    #    The replacement string contains \n which sed will interpret
    #    as new‑lines, and then printf '%b' will turn those \n into
    #    actual new‑lines in the output file.
    awk -v env="$ENV_JSON" '
     { gsub(/"__ENV__"/, env) }
     1
    ' "$TEMPLATE" > "$DEST"
fi
    

if [ ! -f ./.vscode/settings.json ]; then
    cp ./hacks/vscode/settings.json ./.vscode/settings.json;
fi

if [ ! -f "$DEBUG_SH" ]; then
    export_lines=$(
      awk -F= '
        /^[^#]/ {
          gsub(/"/, "\\\"", $2);
          printf "export \"%s\"=\"%s\"\n", $1, $2
        }
      ' "$ENV_FILE"
    )

  # Write the shebang
  printf '#!/usr/bin/bash\n' > "$DEBUG_SH"

  # Append the exported envs
  printf '%s' "$export_lines" >> "$DEBUG_SH"

  printf '\n' >> "$DEBUG_SH"

  # Append the command that starts the API
  printf 'go run cmd/api/main.go\n' >> "$DEBUG_SH"

  # Make the helper script executable
  chmod +x "$DEBUG_SH"
fi
