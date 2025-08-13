#!/bin/bash
find . -type d -not -path "*/.*" -print0 | while IFS= read -r -d $'\0' dir; do
  if [ ! -f "$dir/go.mod" ]; then
    echo "Обработка директории: $dir"
    (cd "$dir" && go mod init)
    (cd "$dir" && go mod tidy)
  fi
done
