module Greeting
  PREFIX = "hi"
  def greet
    puts PREFIX
  end
end

class Animal
  def initialize(name)
    @name = name
  end
  def name
    @name
  end
  def sound
    "..."
  end
  def speak
    puts sound
  end
  def self.kind
    "animal"
  end
end

class Dog < Animal
  def sound
    "woof"
  end
  def speak
    super
    puts @name
  end
end

class Person
  include Greeting
  COUNT = 0
  def initialize(name)
    @name = name
  end
end

a = Animal.new("cat")
a.speak
d = Dog.new("rex")
d.speak
puts d.name
puts Animal.kind
puts Dog.superclass
p = Person.new("ann")
p.greet
puts Person::COUNT
