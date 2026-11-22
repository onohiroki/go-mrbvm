p nil.class
p true.class
p false.class
p NilClass.method_defined?(:new)
p TrueClass.method_defined?(:new)
p FalseClass.method_defined?(:new)

p(nil & true)
p(nil | true)
p(nil | false)
p(nil ^ true)
p(nil ^ false)
p(true & true)
p(true & false)
p(true | false)
p(true ^ true)
p(false | true)
p(false ^ true)

p nil.to_s
p true.to_s
p false.to_s
p nil.nil?
p true.nil?
p false.nil?

p(nil&.size)
p([]&.size)
