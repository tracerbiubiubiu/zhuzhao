-- Roll back only with code no longer depending on maintenance admission facts.
-- Export these authorization/audit facts before removal when retention applies.
DROP TABLE maintenance_task_admissions;
