#!/usr/bin/env sh
# 新建一对迁移文件，默认是业务迁移（server/migrations/app/NNNNNN_<name>.up.sql / .down.sql）。
# 业务迁移和脚手架迁移（server/migrations）各自编号，合并上游时不会冲突；
# 只有修改脚手架本身时才用 TRACK=base。
# 名称以 create_ 开头时，up 文件是一张业务表的模板，已带上标准字段
# （见 docs/table-guide.md），改好业务字段和注释后运行 make migrate-up。
#
#   make new-migration NAME=create_biz_orders
#   make new-migration NAME=add_sys_x TRACK=base
set -eu

name="${1:-}"
track="${2:-app}"
case "$name" in
  '' | *[!a-z0-9_]* | [!a-z]*)
    echo "用法：make new-migration NAME=create_biz_orders（小写字母开头，只含小写字母、数字、下划线）" >&2
    exit 2
    ;;
esac

case "$track" in
  app) dir="$(cd "$(dirname "$0")/.." && pwd)/server/migrations/app" ;;
  base) dir="$(cd "$(dirname "$0")/.." && pwd)/server/migrations" ;;
  *)
    echo "TRACK 只能是 app（默认，业务迁移）或 base（脚手架迁移）" >&2
    exit 2
    ;;
esac
mkdir -p "$dir"
last=$(ls "$dir" | sed -n 's/^\([0-9]\{6\}\)_.*\.up\.sql$/\1/p' | sort | tail -n 1)
next=$(printf '%06d' "$(expr "${last:-0}" + 1)")
up="$dir/${next}_${name}.up.sql"
down="$dir/${next}_${name}.down.sql"

table="${name#create_}"
if [ "$table" != "$name" ]; then
  cat > "$up" <<SQL
-- ${table}：<用途>。规范见 docs/table-guide.md。

CREATE TABLE ${table} (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,

    -- 业务字段：按需修改。每个字段都写 COMMENT（中文名，生成器用作标签）。
    name       varchar(64)  NOT NULL,
    type       smallint     NOT NULL DEFAULT 1,
    remark     varchar(255) NOT NULL DEFAULT '',

    -- 可选的约定字段：不需要就删掉
    status     smallint     NOT NULL DEFAULT 1 CHECK (status IN (1, 2)), -- 启用/停用
    sort       integer      NOT NULL DEFAULT 0,                          -- 排序
    dept_id    bigint REFERENCES sys_departments (id),                   -- 按部门做数据权限

    -- 标准字段：保留
    version    integer      NOT NULL DEFAULT 0,
    created_at timestamptz  NOT NULL DEFAULT now(),
    updated_at timestamptz  NOT NULL DEFAULT now(),
    created_by bigint,
    updated_by bigint
);

COMMENT ON TABLE ${table} IS '<中文表名>';
COMMENT ON COLUMN ${table}.name IS '名称';
-- 写成「中文名：值=文字,值=文字」，生成器会做成下拉框并校验取值
COMMENT ON COLUMN ${table}.type IS '类型：1=普通,2=重要';
COMMENT ON COLUMN ${table}.remark IS '备注';
COMMENT ON COLUMN ${table}.dept_id IS '所属部门';

-- 按常用查询建索引，唯一约束写成 UNIQUE 索引（重复时生成的代码会提示「xx 已存在」）
-- CREATE UNIQUE INDEX ${table}_name_uq ON ${table} (name);
SQL
  echo "DROP TABLE IF EXISTS ${table};" > "$down"
else
  printf -- '-- %s\n' "$name" > "$up"
  printf -- '-- 回滚 %s\n' "$name" > "$down"
fi

echo "已创建："
echo "  ${up#"$(pwd)"/}"
echo "  ${down#"$(pwd)"/}"
