DELETE FROM menu_apis WHERE (api_path = '/api/v1/tickets/transition' AND api_method = 'POST')
   OR (api_path = '/api/v1/tickets/:id/transitions/available' AND api_method = 'GET');
