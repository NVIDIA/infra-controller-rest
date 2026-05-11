-- Reverts component.rack_id back to NOT NULL. This will fail if any
-- component rows exist with NULL rack_id; those must be deleted or
-- assigned to a rack before downgrading.
ALTER TABLE component ALTER COLUMN rack_id SET NOT NULL;
