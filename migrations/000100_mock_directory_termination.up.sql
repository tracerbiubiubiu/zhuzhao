-- Keep explicit directory lifecycle authority separate from local account status.
ALTER TABLE directory_mock_employee_bindings
    ADD COLUMN source_terminated boolean NOT NULL DEFAULT false;
