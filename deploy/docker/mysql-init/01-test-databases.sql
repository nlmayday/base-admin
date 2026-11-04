-- Integration tests create and drop one database per test, named test_<hex>
-- (internal/platform/testinfra). Development only.
GRANT ALL PRIVILEGES ON `test\_%`.* TO 'admin'@'%';
