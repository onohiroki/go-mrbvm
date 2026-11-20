i = 0
sum = 0
while i < 5
  i = i + 1
  if i == 2
    next
  end
  if i == 5
    break
  end
  sum = sum + i
end
puts sum
