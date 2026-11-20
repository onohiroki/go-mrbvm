def greet(name)
  puts "hello, #{name}"
end

%w[alice bob].each do |name|
  greet(name)
end
