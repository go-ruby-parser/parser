
begin
  case 5
  in 6
    puts "no"
  end
rescue StandardError => e
  puts "caught #{e.class}"
end