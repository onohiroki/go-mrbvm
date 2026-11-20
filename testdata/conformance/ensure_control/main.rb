def f
  begin
    return 1
  ensure
    puts "ensure return"
  end
end
p f

i = 0
while i < 3
  i = i + 1
  begin
    next if i == 1
    break if i == 2
    puts i
  ensure
    puts "ensure #{i}"
  end
end
puts "done"

def g
  begin
    raise "x"
  ensure
    puts "ensure raise"
  end
rescue => e
  puts e.message
end
g
