DELETE FROM menu_apis WHERE api_path LIKE '/api/v1/assignment-rules%' AND api_method IN ('GET', 'POST');
