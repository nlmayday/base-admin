-- 菜单管理 used the online icon ep:menu, which loads from api.iconify.design
-- and stays blank on an intranet. Switch it to the bundled Remix icon unless
-- an administrator has already changed it.
UPDATE sys_menus SET icon = 'ri:layout-grid-line' WHERE code = 'iam.menu' AND icon = 'ep:menu';
