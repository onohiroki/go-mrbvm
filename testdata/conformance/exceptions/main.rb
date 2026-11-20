begin
  puts "body"
  raise "boom"
rescue ArgumentError
  puts "arg"
rescue => e
  puts e.message
else
  puts "ok"
ensure
  puts "ensure"
end
puts "after"

def method_rescue
  raise "method"
rescue => e
  "caught #{e.message}"
end
p method_rescue

class MyError < StandardError
end
begin
  raise MyError, "custom"
rescue ArgumentError
  puts "wrong"
rescue MyError => e
  puts e.message
end

x = (raise "modifier" rescue 5)
p x

begin
  puts "normal"
rescue
  puts "rescue"
else
  puts "else"
ensure
  puts "ensure-only"
end

begin
  raise MyError, "multi"
rescue ArgumentError, MyError => e
  puts e.message
end
