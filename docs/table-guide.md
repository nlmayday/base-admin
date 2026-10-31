# 建表规范

业务表按这份规范建，代码生成器就能直接生成出可用的增删改查、搜索、下拉和数据权限，基本不用手改。

## 1. 流程

```bash
make new-migration NAME=create_biz_orders   # 生成 server/migrations/NNNNNN_create_biz_orders.{up,down}.sql，up 文件已带好标准字段
# 修改业务字段与注释
make migrate-up
```

然后在「开发工具 → 代码生成」中选表生成。表结构只能通过迁移修改，不要在数据库里手工改。

## 2. 命名

- 表名、列名用小写字母、数字和下划线，不要用 SQL 关键字（`order`、`user`、`group` 等）。
- 业务表用 `biz_` 或模块名做前缀，如 `biz_orders`、`crm_customers`；`sys_` 留给系统表。
- 外键列写成 `<对象>_id`，如 `customer_id`。

## 3. 字段

### 必须有

| 字段 | 定义 | 说明 |
|---|---|---|
| `id` | `bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY` | 不支持复合主键和 UUID 主键 |

### 标准字段（每张业务表都加）

| 字段 | 定义 | 没有它会怎样 |
|---|---|---|
| `version` | `integer NOT NULL DEFAULT 0` | 编辑时发现不了他人已修改（乐观锁），后保存的会覆盖先保存的 |
| `created_at` | `timestamptz NOT NULL DEFAULT now()` | 不能按创建时间排序、筛选 |
| `updated_at` | `timestamptz NOT NULL DEFAULT now()` | 不知道最后修改时间 |
| `created_by` | `bigint` | 不记录创建人，数据权限「仅本人」不生效 |
| `updated_by` | `bigint` | 不记录修改人 |

这五个字段由系统维护，不会出现在表单里。缺少时，生成预览会在提示中列出来。

### 约定字段（按需加，名字和类型要一致才会被识别）

| 字段 | 定义 | 生成器的处理 |
|---|---|---|
| `status` | `smallint NOT NULL DEFAULT 1 CHECK (status IN (1, 2))` | 启用/停用：表单单选、列表标签、搜索下拉 |
| `sort` | `integer NOT NULL DEFAULT 0` | 列表按它排序 |
| `dept_id` | `bigint REFERENCES sys_departments (id)` | 部门树选择；按部门数据权限过滤 |
| `deleted_at` | `timestamptz`（可为空） | 软删除：删除时只记录时间，列表不再显示 |

## 4. 类型

| 用途 | 类型 | 页面控件 |
|---|---|---|
| 短文本（名称、编码） | `varchar(n)` | 输入框，限制长度 |
| 长文本（备注、说明） | `text` 或 `varchar(n>255)` | 多行输入；可为空的长文本默认不在列表显示 |
| 整数 | `smallint` / `integer` / `bigint` | 数字输入 |
| 金额 | `numeric(12,2)` | 金额输入，可按范围搜索 |
| 是/否 | `boolean` | 开关 |
| 日期 | `date` | 日期选择 |
| 时间 | `timestamptz` | 日期时间选择 |
| 关联（外键） | `bigint REFERENCES 目标表 (id)` | 可搜索的下拉，列表和导出显示名称（默认取目标表的 name / title / nickname 等列，可在代码生成页改）；保存时校验记录存在，被引用的记录不能删除。不加外键约束时只是普通数字输入 |
| 图片（封面、头像） | `varchar(512) NOT NULL DEFAULT ''` | 图片上传，列表显示缩略图。字段名形如 `cover_url`、`avatar`、`image`，或注释含「图片、封面、头像、照片」时自动使用 |
| 附件列表 | `jsonb NOT NULL DEFAULT '[]'` | 附件上传，列表显示个数。字段名为 `attachments`、`files`，或注释含「附件」时自动使用 |
| JSON | `jsonb` | 文本框，校验 JSON |

## 5. 必填与默认值

- `NOT NULL` 且没有默认值：表单必填。
- 字面量默认值（`DEFAULT 0`、`DEFAULT 'normal'`）：表单预填，可以修改。
- 函数默认值（`now()`、`gen_random_uuid()`）：由数据库生成，不出现在表单里。
- 可为空（不写 `NOT NULL`）：表单选填。

## 6. 注释（中文名与下拉选项）

每个业务字段都写注释，生成器用注释作为表头、表单和搜索项的中文名：

```sql
COMMENT ON TABLE biz_orders IS '订单';
COMMENT ON COLUMN biz_orders.order_no IS '订单号';
COMMENT ON COLUMN biz_orders.remark IS '备注，客户的特殊要求';  -- 中文名取逗号前的「备注」
```

字段取值是固定几项时，把选项写进注释，生成器会做成下拉框，列表显示文字，后端只接受这些值：

```sql
COMMENT ON COLUMN biz_orders.type IS '类型：1=普通,2=加急';
COMMENT ON COLUMN biz_orders.channel IS '渠道（web=网页，app=应用，api=接口）';
```

- 格式：`中文名：值=文字,值=文字`，冒号也可以换成括号；选项之间用逗号、分号、顿号或空格分隔。
- 值只能是字母、数字、`_`、`-`（最长 32 位），文字最长 20 个字；至少两项。格式不对时按普通输入框处理。
- 这些选项写进生成的代码，以后要改需要改代码。如果选项需要管理员在页面上维护（如客户级别），就不要写在注释里，而是在「系统配置 → 数据字典」中建字典，在代码生成页的「下拉选项」里选这个字典。
- 代码生成页可以给每个字段选择：不使用 / 注释中的选项 / 某个数据字典。

## 7. 约束与索引

- 不能重复的字段建唯一索引：`CREATE UNIQUE INDEX biz_orders_order_no_uq ON biz_orders (order_no);`。保存重复值时，生成的代码会在该字段下提示「订单号已存在」。有软删除时写成部分索引：`... WHERE deleted_at IS NULL`。
- 金额、数量这类规则用 `CHECK` 约束兜底，如 `CHECK (amount >= 0)`。
- 常用的搜索、排序字段建索引。

## 8. 完整示例

```sql
CREATE TABLE biz_orders (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    order_no    varchar(32)   NOT NULL,
    customer    varchar(64)   NOT NULL,
    type        smallint      NOT NULL DEFAULT 1,
    amount      numeric(12,2) NOT NULL DEFAULT 0 CHECK (amount >= 0),
    order_date  date          NOT NULL,
    paid        boolean       NOT NULL DEFAULT false,
    remark      text,
    status      smallint      NOT NULL DEFAULT 1 CHECK (status IN (1, 2)),
    dept_id     bigint REFERENCES sys_departments (id),
    version     integer       NOT NULL DEFAULT 0,
    created_at  timestamptz   NOT NULL DEFAULT now(),
    updated_at  timestamptz   NOT NULL DEFAULT now(),
    created_by  bigint,
    updated_by  bigint
);

COMMENT ON TABLE biz_orders IS '订单';
COMMENT ON COLUMN biz_orders.order_no IS '订单号';
COMMENT ON COLUMN biz_orders.customer IS '客户名称';
COMMENT ON COLUMN biz_orders.type IS '类型：1=普通,2=加急';
COMMENT ON COLUMN biz_orders.amount IS '金额';
COMMENT ON COLUMN biz_orders.order_date IS '下单日期';
COMMENT ON COLUMN biz_orders.paid IS '是否已付款';
COMMENT ON COLUMN biz_orders.remark IS '备注';
COMMENT ON COLUMN biz_orders.dept_id IS '所属部门';

CREATE UNIQUE INDEX biz_orders_order_no_uq ON biz_orders (order_no);
```
