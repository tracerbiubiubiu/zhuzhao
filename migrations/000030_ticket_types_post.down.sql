-- down：五行回原 PUT/DELETE path（复合定位同红线）

UPDATE menu_apis SET api_path = '/api/v1/ticket-types/:code', api_method = 'PUT'
WHERE api_path = '/api/v1/ticket-types/update' AND api_method = 'POST';

UPDATE menu_apis SET api_path = '/api/v1/ticket-types/:code', api_method = 'DELETE'
WHERE api_path = '/api/v1/ticket-types/delete' AND api_method = 'POST';

UPDATE menu_apis SET api_path = '/api/v1/ticket-types/:code/fields', api_method = 'PUT'
WHERE api_path = '/api/v1/ticket-types/fields/replace' AND api_method = 'POST';

UPDATE menu_apis SET api_path = '/api/v1/ticket-templates/:code', api_method = 'PUT'
WHERE api_path = '/api/v1/ticket-templates/update' AND api_method = 'POST';

UPDATE menu_apis SET api_path = '/api/v1/ticket-templates/:code', api_method = 'DELETE'
WHERE api_path = '/api/v1/ticket-templates/delete' AND api_method = 'POST';
