e = [1, 2, 3].each
p e.class
p e.to_a
p e.map { |x| x * 2 }
p e.select { |x| x > 1 }
p e.next
p e.next
p e.peek
p e.next
begin
  e.next
rescue StopIteration
  p :stop
end
e.rewind
p e.next

p [1, 2].each_with_index.to_a
p [1, 2, 3].each_slice(2).to_a
p [1, 2, 3].each_cons(2).to_a
p (1..3).each.to_a
p ({a: 1, b: 2}).each.to_a
p [1, 2].each.with_index.to_a rescue p :no_with_index
p [1, 2].map.class
p [1, 2].select.class
p [1, 2].each.size
