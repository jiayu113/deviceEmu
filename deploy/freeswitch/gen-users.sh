#!/bin/sh
# 用法: gen-users.sh <起> <止>
# 以 vanilla 1000.xml 为模板,生成 [起,止] 区间的分机 xml(密码沿用 $${default_password})。
set -eu
DIR=/usr/share/freeswitch/conf/vanilla/directory/default
start="$1"; end="$2"
[ -f "$DIR/1000.xml" ] || { echo "template $DIR/1000.xml missing"; exit 1; }
for i in $(seq "$start" "$end"); do
  [ "$i" = "1000" ] && continue            # 1000 已存在,跳过
  sed "s/1000/$i/g" "$DIR/1000.xml" > "$DIR/$i.xml"
done
echo "generated extensions $start..$end"