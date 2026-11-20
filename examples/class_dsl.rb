# A small class-based DSL in the supported subset.
class Task
  def initialize(name, deps = [])
    @name = name
    @deps = deps
  end

  def each_dep
    @deps.each { |dep| yield dep }
  end

  def describe
    puts "task #{@name}"
    each_dep { |dep| puts "  depends on #{dep}" }
  end

  def self.kind
    "task"
  end
end

class Build < Task
  def describe
    super
    puts "  compile"
  end
end

Build.new("app", ["lib", "tools"]).describe
puts Build.kind
