#!/usr/bin/bash

# 1.  Paths 
TEMPLATE="./hacks/vscode/launch.json"  
DEST="./.vscode/launch.json"           

if [ ! -d ./.vscode ]; then
    mkdir -p ./.vscode;
fi

if [ ! -f "$DEST" ]; then
    cp "$TEMPLATE" "$DEST"
fi
    

if [ ! -f ./.vscode/settings.json ]; then
    cp ./hacks/vscode/settings.json ./.vscode/settings.json;
fi
