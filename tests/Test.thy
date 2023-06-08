theory Test

imports Main

begin

fun sum where
  "sum 0 = 0" |
  "sum (Suc n) = (Suc n) + sum n"

lemma "sum (n::nat) = n * (n+1) div 2"
proof (induct n)
  case 0
  then show ?case by simp
next
  case (Suc n)
  then show ?case sorry
qed

end
