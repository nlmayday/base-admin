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
