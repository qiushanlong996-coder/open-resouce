CREATE TABLE featured_projects (
    project_id VARCHAR(64) NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    created_by VARCHAR(64) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (project_id),
    INDEX idx_featured_projects_order (sort_order, project_id),
    CONSTRAINT fk_featured_projects_project
        FOREIGN KEY (project_id) REFERENCES managed_projects (id)
        ON DELETE CASCADE ON UPDATE RESTRICT
) ENGINE = InnoDB
  DEFAULT CHARACTER SET = utf8mb4
  COLLATE = utf8mb4_unicode_ci;
