CREATE INDEX IF NOT EXISTS staff_members_status_created_id_idx
    ON organization.staff_members (employment_status, created_at DESC, id);

CREATE INDEX IF NOT EXISTS staff_import_batches_created_at_idx
    ON organization.staff_import_batches (created_at DESC, id);

CREATE INDEX IF NOT EXISTS departments_name_lower_idx
    ON organization.departments (lower(name));
