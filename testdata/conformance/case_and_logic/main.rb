x = 3
case x
when 1
  puts "one"
when 2..4
  puts "range"
else
  puts "other"
end
a = nil || 5
b = false && 6
puts a
p b
puts !nil
