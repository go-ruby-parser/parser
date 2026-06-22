class A
end
class B < A
  def g
    super
  end
end
B.new.g