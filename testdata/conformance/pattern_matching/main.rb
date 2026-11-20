x = 7
case x
in Integer => n if n > 5
  puts "big #{n}"
else
  puts "nope"
end

expected = 3
case 3
in ^expected
  puts "pinned"
else
  puts "not pinned"
end

case [1, [2, 3]]
in [a, [b, c]]
  puts a + b + c
else
  puts "array no"
end

case [1, 2, 3]
in [a, b]
  puts a
else
  puts "size mismatch"
end

case 1
in 1
  puts "matched"
end

