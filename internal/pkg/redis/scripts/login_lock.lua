-- KEYS[1] = lock:login:{employee_no}
-- ARGV[1] = window_sec (900)
-- ARGV[2] = max_fail   (5)
local n = redis.call('INCR', KEYS[1])
if n == 1 then
  redis.call('EXPIRE', KEYS[1], tonumber(ARGV[1]))
end
if n > tonumber(ARGV[2]) then
  return 1
end
return 0
