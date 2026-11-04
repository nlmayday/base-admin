DROP TABLE IF EXISTS sys_notice_reads;
DROP TABLE IF EXISTS sys_notices;
DELETE FROM sys_dict_types WHERE code = 'notice_type';
