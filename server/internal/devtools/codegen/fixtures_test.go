package codegen

// customerTable mirrors the planned example table biz_customers.
func customerTable() Table {
	return Table{Name: "biz_customers", Comment: "客户", Columns: []Column{
		{Name: "id", UDT: "int8", Identity: true, Primary: true},
		{Name: "name", UDT: "varchar", MaxLength: 64, Comment: "客户名称，必填"},
		{Name: "contact", UDT: "varchar", MaxLength: 32, Default: "''::character varying"},
		{Name: "phone", UDT: "varchar", MaxLength: 32, Default: "''::character varying", Unique: true},
		{Name: "level", UDT: "varchar", MaxLength: 16, Default: "'normal'::character varying", Comment: "客户级别（字典 customer_level）"},
		{Name: "dept_id", UDT: "int8", Nullable: true},
		{Name: "owner_id", UDT: "int8", Nullable: true, Comment: "负责人"},
		{Name: "status", UDT: "int2", Default: "1"},
		{Name: "sort", UDT: "int4", Default: "0"},
		{Name: "remark", UDT: "varchar", MaxLength: 255, Default: "''::character varying"},
		{Name: "version", UDT: "int4", Default: "0"},
		{Name: "created_at", UDT: "timestamptz", Default: "now()"},
		{Name: "updated_at", UDT: "timestamptz", Default: "now()"},
		{Name: "created_by", UDT: "int8", Nullable: true},
		{Name: "updated_by", UDT: "int8", Nullable: true},
		{Name: "deleted_at", UDT: "timestamptz", Nullable: true},
	}}
}

// minimalTable has none of the recognised columns.
func minimalTable() Table {
	return Table{Name: "biz_notes", Columns: []Column{
		{Name: "id", UDT: "int8", Default: "nextval('biz_notes_id_seq'::regclass)", Primary: true},
		{Name: "body", UDT: "text"},
	}}
}

// mixedTable exercises every supported type and nullability.
func mixedTable() Table {
	return Table{Name: "biz_orders", Columns: []Column{
		{Name: "id", UDT: "int8", Identity: true, Primary: true},
		{Name: "code", UDT: "uuid", Default: "gen_random_uuid()"},
		{Name: "order", UDT: "int4", Nullable: true},
		{Name: "amount", UDT: "numeric"},
		{Name: "discount", UDT: "numeric", Nullable: true},
		{Name: "ratio", UDT: "float8", Nullable: true},
		{Name: "paid", UDT: "bool", Default: "true"},
		{Name: "archived", UDT: "bool", Nullable: true},
		{Name: "due_on", UDT: "date"},
		{Name: "shipped_at", UDT: "timestamptz", Nullable: true},
		{Name: "meta", UDT: "jsonb", Nullable: true},
		{Name: "client_ip", UDT: "inet", Nullable: true},
		{Name: "customer_id", UDT: "int8"},
		{Name: "big", UDT: "int8", Nullable: true},
		{Name: "dept_id", UDT: "int8"},
		{Name: "status", UDT: "int4", Default: "1"},
		{Name: "title", UDT: "bpchar", MaxLength: 8, Nullable: true},
		{Name: "tags", UDT: "_text", Nullable: true},
		{Name: "password_hash", UDT: "text", Default: "''"},
		{Name: "created_at", UDT: "timestamptz"},
	}}
}

// ticketTable has options written in column comments.
func ticketTable() Table {
	return Table{Name: "biz_tickets", Columns: []Column{
		{Name: "id", UDT: "int8", Identity: true, Primary: true},
		{Name: "title", UDT: "varchar", MaxLength: 128, Comment: "标题"},
		{Name: "kind", UDT: "int2", Default: "1", Comment: "类型：1=通知,2=公告"},
		{Name: "source", UDT: "varchar", MaxLength: 16, Nullable: true, Comment: "来源（web=网页，app=应用）"},
		{Name: "created_at", UDT: "timestamptz", Default: "now()"},
	}}
}

// productTable has image and attachment columns.
func productTable() Table {
	return Table{Name: "biz_products", Columns: []Column{
		{Name: "id", UDT: "int8", Identity: true, Primary: true},
		{Name: "name", UDT: "varchar", MaxLength: 64, Comment: "名称"},
		{Name: "cover_url", UDT: "varchar", MaxLength: 512, Default: "''::character varying", Comment: "封面"},
		{Name: "photo", UDT: "text", Nullable: true, Comment: "实拍图"},
		{Name: "attachments", UDT: "jsonb", Default: "'[]'::jsonb", Comment: "附件"},
		{Name: "specs", UDT: "jsonb", Nullable: true, Comment: "规格参数"},
		{Name: "version", UDT: "int4", Default: "0"},
		{Name: "created_at", UDT: "timestamptz", Default: "now()"},
	}}
}

// dealTable has foreign keys: required, nullable (soft-delete target) and
// one without a label column.
func dealTable() Table {
	return Table{Name: "biz_deals", Columns: []Column{
		{Name: "id", UDT: "int8", Identity: true, Primary: true},
		{Name: "title", UDT: "varchar", MaxLength: 64, Comment: "标题"},
		{Name: "customer_id", UDT: "int8", Comment: "客户", Ref: &Ref{Table: "biz_customers", Labels: []string{"name", "phone"}}},
		{Name: "owner_id", UDT: "int8", Nullable: true, Comment: "负责人", Ref: &Ref{Table: "sys_users", Labels: []string{"nickname", "username"}, SoftDelete: true}},
		{Name: "batch_id", UDT: "int8", Nullable: true, Comment: "批次", Ref: &Ref{Table: "biz_batches"}},
		{Name: "version", UDT: "int4", Default: "0"},
		{Name: "created_at", UDT: "timestamptz", Default: "now()"},
	}}
}

// playerTable is a table of another service: a snowflake bigint id the
// program assigns, no standard columns.
func playerTable() Table {
	return Table{Name: "lg_player", Comment: "玩家", Columns: []Column{
		{Name: "id", UDT: "int8", Primary: true},
		{Name: "nickname", UDT: "varchar", MaxLength: 32},
		{Name: "level", UDT: "int4", Default: "1"},
		{Name: "price_currency", UDT: "bpchar", MaxLength: 3, Default: "'USD'::bpchar", Comment: "币种"},
		{Name: "user_id", UDT: "int8", Nullable: true, Comment: "账号"},
		{Name: "created_at", UDT: "timestamptz", Default: "now()"},
	}}
}

// configTable has a text natural key.
func configTable() Table {
	return Table{Name: "cfg_game", Comment: "游戏配置", Columns: []Column{
		{Name: "cfg_key", UDT: "varchar", MaxLength: 64, Primary: true, Comment: "配置键"},
		{Name: "cfg_value", UDT: "text", Comment: "配置值"},
		{Name: "remark", UDT: "varchar", MaxLength: 255, Nullable: true},
		{Name: "version", UDT: "int4", Default: "0"},
	}}
}

// levelTable has an integer natural key.
func levelTable() Table {
	return Table{Name: "cfg_level", Comment: "等级", Columns: []Column{
		{Name: "level", UDT: "int4", Primary: true, Comment: "等级"},
		{Name: "exp", UDT: "int8", Comment: "所需经验"},
		{Name: "title", UDT: "varchar", MaxLength: 32},
	}}
}

// orderLogTable is a ledger, generated read-only.
func orderLogTable() Table {
	return Table{Name: "lg_order_log", Comment: "订单流水", Columns: []Column{
		{Name: "id", UDT: "int8", Primary: true},
		{Name: "order_no", UDT: "varchar", MaxLength: 32, Comment: "订单号"},
		{Name: "player_id", UDT: "int8", Comment: "玩家", Ref: &Ref{Table: "lg_player", Labels: []string{"nickname"}}},
		{Name: "amount", UDT: "numeric", Comment: "金额"},
		{Name: "status", UDT: "int2", Default: "1", Comment: "状态：1=待支付,2=已支付"},
		{Name: "paid_at", UDT: "timestamptz", Nullable: true},
		{Name: "updated_by", UDT: "int8", Nullable: true},
		{Name: "created_at", UDT: "timestamptz", Default: "now()"},
	}}
}

// keyShapes are the key and read-only cases of the compile and web checks.
func keyShapes(prefix string) []struct {
	table Table
	spec  Spec
} {
	return []struct {
		table Table
		spec  Spec
	}{
		{playerTable(), Spec{Module: prefix + "player", Resource: "player", Name: "玩家", NewModule: true}},
		{configTable(), Spec{Module: prefix + "cfg", Resource: "game-config", Name: "游戏配置", NewModule: true,
			Fields: []FieldOption{{Column: "cfg_key", Search: SearchEq}}}},
		{levelTable(), Spec{Module: prefix + "level", Resource: "level", Name: "等级", NewModule: true}},
		{itemTable(), itemSpec(prefix + "item")},
		{bannerTable(), Spec{Module: prefix + "banner", Resource: "banner", Name: "公告横幅", NewModule: true, Children: []ChildSpec{bannerChild()}}},
		{vipLevelTable(), Spec{Module: prefix + "vip", Resource: "vip-level", Name: "VIP 等级", NewModule: true}},
		{playerTable(), Spec{Module: prefix + "players", Resource: "player", Name: "玩家", NewModule: true, ReadOnly: true}},
		{orderLogTable(), Spec{Module: prefix + "ledger", Resource: "order-log", Name: "订单流水", NewModule: true, ReadOnly: true,
			Fields: []FieldOption{{Column: "order_no", Search: SearchEq}, {Column: "paid_at", Search: SearchRange}}}},
	}
}

// itemTable is a main table with a translation child table keyed by
// (item_id, lang).
func itemTable() Table {
	return Table{Name: "lg_item", Comment: "道具", Columns: []Column{
		{Name: "id", UDT: "int8", Primary: true},
		{Name: "code", UDT: "varchar", MaxLength: 32, Unique: true, Comment: "编码"},
		{Name: "price", UDT: "int4", Default: "0", Comment: "价格"},
	}, Children: []ChildTable{{
		Spec: ChildSpec{Table: "lg_item_i18n", ForeignKey: "item_id", KeyColumn: "lang", KeyDict: "lang", Label: "多语言"},
		Table: Table{Name: "lg_item_i18n", Columns: []Column{
			{Name: "item_id", UDT: "int8", Primary: true},
			{Name: "lang", UDT: "varchar", MaxLength: 8, Primary: true, Comment: "语言"},
			{Name: "name", UDT: "varchar", MaxLength: 64, Comment: "名称"},
			{Name: "description", UDT: "text", Nullable: true, Comment: "描述"},
			{Name: "updated_at", UDT: "timestamptz", Default: "now()"},
		}},
	}}}
}

// itemSpec generates itemTable with its child.
func itemSpec(module string) Spec {
	return Spec{Module: module, Resource: "item", Name: "道具", NewModule: true,
		Children: []ChildSpec{itemTable().Children[0].Spec}}
}

// vipLevelTable has no column that is searched by default.
func vipLevelTable() Table {
	return Table{Name: "lg_vip_level", Comment: "VIP 等级", Columns: []Column{
		{Name: "level", UDT: "int4", Primary: true, Comment: "等级"},
		{Name: "min_exp", UDT: "int8", Comment: "最低经验"},
		{Name: "daily_bonus", UDT: "int4", Default: "0", Comment: "每日奖励"},
	}}
}

// bannerTable has a translation child keyed by a language table that the
// game server shares (keys with required ones), and its own child id.
func bannerTable() Table {
	return Table{Name: "lg_banner", Comment: "公告横幅", Columns: []Column{
		{Name: "id", UDT: "int8", Primary: true},
		{Name: "sort", UDT: "int4", Default: "0"},
	}, Children: []ChildTable{{Spec: bannerChild(), Table: Table{Name: "lg_banner_i18n", Columns: []Column{
		{Name: "id", UDT: "int8", Identity: true, Primary: true},
		{Name: "banner_id", UDT: "int8"},
		{Name: "lang", UDT: "varchar", MaxLength: 8, Comment: "语言"},
		{Name: "title", UDT: "varchar", MaxLength: 64, Comment: "标题"},
		{Name: "image_url", UDT: "varchar", MaxLength: 512, Nullable: true, Comment: "图片"},
	}}}}}
}

func bannerChild() ChildSpec {
	return ChildSpec{Table: "lg_banner_i18n", ForeignKey: "banner_id", KeyColumn: "lang", Label: "多语言",
		KeyTable: "lg_language", KeyValueColumn: "code", KeyLabelColumn: "name", KeyFilter: "enabled = 1",
		KeySortColumn: "sort", KeyRequiredColumn: "required"}
}
