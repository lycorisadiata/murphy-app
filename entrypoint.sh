#!/bin/sh
set -e

# 目标挂载目录
DATA_DIR="/anheyu/data"
DEFAULT_DIR="/anheyu/default-data"

# DefaultArticle.md 填充
if [ ! -f "$DATA_DIR/DefaultArticle.md" ] && [ -f "$DEFAULT_DIR/DefaultArticle.md" ]; then
  cp -f "$DEFAULT_DIR/DefaultArticle.md" "$DATA_DIR/DefaultArticle.md"
  echo "[entrypoint] Seeded DefaultArticle.md to $DATA_DIR/DefaultArticle.md"
fi

# DefaultArticle.html 填充（预渲染的 HTML 内容）
if [ ! -f "$DATA_DIR/DefaultArticle.html" ] && [ -f "$DEFAULT_DIR/DefaultArticle.html" ]; then
  cp -f "$DEFAULT_DIR/DefaultArticle.html" "$DATA_DIR/DefaultArticle.html"
  echo "[entrypoint] Seeded DefaultArticle.html to $DATA_DIR/DefaultArticle.html"
fi

# conf.ini 填充
if [ ! -f "$DATA_DIR/conf.ini" ] && [ -f "$DEFAULT_DIR/conf.ini" ]; then
  cp -f "$DEFAULT_DIR/conf.ini" "$DATA_DIR/conf.ini"
  echo "[entrypoint] Seeded conf.ini to $DATA_DIR/conf.ini"
fi

exec "$@"


