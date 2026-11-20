n = 0
begin
  n = n + 1
  raise "retry" if n < 3
  puts "ok"
rescue
  retry
end
p n

i = 0
while i < 2
  i = i + 1
  puts "w#{i}"
  redo if i == 1
end

m = 0
[1].each do
  m = m + 1
  redo if m == 1
end
p m

k = 0
for x in [1]
  k = k + 1
  redo if k == 1
end
p k
