-- 108_seed_content_blocks.down.sql

BEGIN;

DELETE FROM platform_admin.content_blocks
WHERE key IN ('about-vision', 'about-mission', 'about-banner', 'home-hero', 'highlight-coldchain', 'highlight-fastdelivery', 'highlight-einvoice');

COMMIT;
