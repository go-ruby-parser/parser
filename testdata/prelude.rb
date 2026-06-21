# frozen_string_literal: true
#
# The embedded-Ruby prelude: standard library pieces that are cleaner to express
# in Ruby than in Go. Loaded once by VM.New after the native bootstrap, so every
# program sees these modules. This is the org's USP — Comparable and Enumerable
# are written *once*, in Ruby, on top of a single primitive each (`<=>` / `each`).

# Comparable derives the ordering operators from `<=>`. A class mixes it in and
# defines `<=>`; everything else follows.
module Comparable
  def <(other)
    (self <=> other) < 0
  end

  def <=(other)
    (self <=> other) <= 0
  end

  def >(other)
    (self <=> other) > 0
  end

  def >=(other)
    (self <=> other) >= 0
  end

  def ==(other)
    (self <=> other) == 0
  end

  def between?(min, max)
    if self < min
      false
    elsif self > max
      false
    else
      true
    end
  end

  def clamp(min, max = nil)
    if min.is_a?(Range)
      raise ArgumentError, "cannot clamp with an exclusive range" if min.exclude_end?
      lo = min.begin
      hi = min.end
      return lo if !lo.nil? && self < lo
      return hi if !hi.nil? && self > hi
      self
    else
      return min if !min.nil? && self < min
      return max if !max.nil? && self > max
      self
    end
  end
end

# Enumerable derives the collection methods from `each`. A class mixes it in and
# defines `each`; map/select/reduce/min/… all follow. (Without break/&& yet, the
# scanning forms below visit every element — correct, if not short-circuiting.)
module Enumerable
  def to_a
    r = []
    each { |x| r << x }
    r
  end

  def map
    r = []
    each { |x| r << yield(x) }
    r
  end

  def count
    n = 0
    each { |x| n = n + 1 if !block_given? || yield(x) }
    n
  end

  # min_by / max_by / sort_by delegate to Array's native implementations via the
  # pair/element list, so any Enumerable (Hash, Range, Struct, …) gains them.
  def min_by
    to_a.min_by { |x| yield(x) }
  end

  def max_by
    to_a.max_by { |x| yield(x) }
  end

  def sort_by
    to_a.sort_by { |x| yield(x) }
  end

  def select
    r = []
    each { |x| r << x if yield(x) }
    r
  end

  def reject
    r = []
    each { |x| r << x unless yield(x) }
    r
  end

  def find
    result = nil
    each { |x|
      if result == nil
        result = x if yield(x)
      end
    }
    result
  end

  def include?(value)
    found = false
    each { |x| found = true if x == value }
    found
  end

  def sum(init = 0)
    total = init
    each { |x| total = total + (block_given? ? yield(x) : x) }
    total
  end

  def min
    result = nil
    first = true
    each { |x|
      if first
        result = x
        first = false
      elsif x < result
        result = x
      end
    }
    result
  end

  def max
    result = nil
    first = true
    each { |x|
      if first
        result = x
        first = false
      elsif x > result
        result = x
      end
    }
    result
  end

  def reduce(*args)
    # Forms: reduce { |a, b| }, reduce(init) { }, reduce(:op), reduce(init, :op).
    sym = nil
    has_init = false
    init = nil
    if args.length == 2
      init = args[0]
      sym = args[1]
      has_init = true
    elsif args.length == 1 && args[0].is_a?(Symbol)
      sym = args[0]
    elsif args.length == 1
      init = args[0]
      has_init = true
    end
    acc = init
    started = has_init
    each do |x|
      if !started
        acc = x
        started = true
      elsif sym
        acc = acc.send(sym, x)
      else
        acc = yield(acc, x)
      end
    end
    acc
  end

  def inject(*args, &blk)
    reduce(*args, &blk)
  end

  def any?
    result = false
    each { |x| result = true if yield(x) }
    result
  end

  def all?
    result = true
    each { |x| result = false unless yield(x) }
    result
  end

  def none?
    result = true
    each { |x| result = false if yield(x) }
    result
  end

  def each_with_index
    i = 0
    each { |x|
      yield(x, i)
      i = i + 1
    }
    self
  end

  def flat_map
    r = []
    each { |x|
      v = yield(x)
      if v.is_a?(Array)
        v.each { |e| r << e }
      else
        r << v
      end
    }
    r
  end

  def each_with_object(memo)
    each { |x| yield(x, memo) }
    memo
  end

  def filter_map
    r = []
    each { |x|
      v = yield(x)
      r << v if v
    }
    r
  end

  def partition
    yes = []
    no = []
    each { |x|
      if yield(x)
        yes << x
      else
        no << x
      end
    }
    [yes, no]
  end

  def group_by
    h = {}
    each { |x|
      k = yield(x)
      (h[k] ||= []) << x
    }
    h
  end

  def tally
    h = {}
    each { |x|
      h[x] = (h[x] || 0) + 1
    }
    h
  end

  def zip(other)
    r = []
    i = 0
    each { |x|
      r << [x, other[i]]
      i = i + 1
    }
    r
  end
end

# The built-in ordered types are Comparable: each defines <=> natively, so they
# pick up <, <=, >, >=, between?, and clamp from the module above. (The
# comparison operators still take the VM's inline fast path; between?/clamp route
# through <=>.)
class Integer
  include Comparable
end

class Float
  include Comparable
end

class Rational
  include Comparable
end

class String
  include Comparable
end

# Array and Range are Enumerable: each defines `each` natively, so they pick up
# select/reject/find/reduce/sum/any?/all?/none?/each_with_index from the module
# above. Their own native methods (map, include?, min, max, count, …) take
# precedence over the module's where both exist. (Hash also wants Enumerable but
# needs block auto-splat for its [k, v] pairs first.)
class Array
  include Enumerable
  # The deconstruct protocol for case/in array patterns: an Array deconstructs
  # to itself.
  def deconstruct
    self
  end
end

class Range
  include Enumerable
end

# Hash is Enumerable too: Hash#each yields a [key, value] pair, so map/find/count
# /any?/all?/none?/to_a operate on pairs. select/reject are native (they return a
# Hash, not an Array).
class Hash
  include Enumerable
  # The deconstruct_keys protocol for case/in hash patterns: a Hash returns
  # itself (the requested key list is advisory, so we ignore it).
  def deconstruct_keys(keys)
    self
  end
end
