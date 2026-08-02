-- 评论 IP 归属地。
--
-- 只存归属地（省级/国家级），不存原始 IP：IP 属于个人信息，落库会带来
-- 长期泄漏风险，而展示所需的只是「发言时人在哪个省」这一时点事实。
-- 归属地在评论写入时算好存下，读路径完全不依赖 IP 库。
--
-- 历史评论没有这个信息（当时未采集 IP），保持空串即可——不做任何回填，
-- 编造地理位置比留空更糟。
ALTER TABLE document_comments
    ADD COLUMN author_region VARCHAR(64) NOT NULL DEFAULT '' AFTER author_name;
