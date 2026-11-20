# A small task DSL in the supported macro subset.
def task(name)
  puts "task: #{name}"
  yield
end

task("build") do
  puts "  compiling"
end

task("test") do
  puts "  running tests"
end
