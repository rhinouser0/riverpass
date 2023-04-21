create table oss_files (
	fid varchar(255) NOT NULL DEFAULT "",
	file_meta json DEFAULT NULL,
    owners varchar(64) DEFAULT "",
    state tinyint(1) NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (fid),
	INDEX owners (owners)
);