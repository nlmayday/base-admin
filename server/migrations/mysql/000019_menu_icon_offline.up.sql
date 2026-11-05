-- MySQL version of ../000019_menu_icon_offline.up.sql.
UPDATE sys_menus SET icon = 'ri:layout-grid-line' WHERE code = 'iam.menu' AND icon = 'ep:menu';
