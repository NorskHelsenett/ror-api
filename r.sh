if [ ! -d ./.vscode ]; then
  mkdir -p ./.vscode;
fi

if [ ! -f ./.vscode/launch.json ]; then
  cp -v ./hacks/vscode/launch.json ./.vscode/launch.json;
fi

if [ ! -f ./.vscode/settings.json ]; then
  cp -v ./hacks/vscode/settings.json ./.vscode/settings.json;
fi
