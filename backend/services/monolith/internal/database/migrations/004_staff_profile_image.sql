ALTER TABLE organization.staff_members
    ADD COLUMN IF NOT EXISTS profile_image_url text;
