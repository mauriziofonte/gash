#!/usr/bin/env bash

# Only run mesg if the shell is interactive and mesg is available
if [[ "$-" == *i* ]] && command -v mesg >/dev/null 2>&1; then
    mesg n 2> /dev/null || true
fi

# run inxi information tool
if [ -x "`which inxi 2>&1`" ]; then
    inxi -IpRS -v0 -c5
fi

# Array of famous IT quotes
QUOTES=(
  "“Talk is cheap. Show me the code.” — Linus Torvalds"
  "“Programs must be written for people to read, and only incidentally for machines to execute.” — Harold Abelson"
  "“Always code as if the guy who ends up maintaining your code will be a violent psychopath who knows where you live.” — John Woods"
  "“Any fool can write code that a computer can understand. Good programmers write code that humans can understand.” — Martin Fowler"
  "“First, solve the problem. Then, write the code.” — John Johnson"
  "“Experience is the name everyone gives to their mistakes.” — Oscar Wilde"
  "“In order to be irreplaceable, one must always be different.” — Coco Chanel"
  "“Java is to JavaScript what car is to Carpet.” — Chris Heilmann"
  "“Knowledge is power.” — Francis Bacon"
  "“Sometimes it pays to stay in bed on Monday, rather than spending the rest of the week debugging Monday’s code.” — Dan Salomon"
  "“Code never lies, comments sometimes do.” — Ron Jeffries"
  "“The only way to go fast, is to go well.” — Robert C. Martin"
  "“Simplicity is the soul of efficiency.” — Austin Freeman"
  "“The best way to get a project done faster is to start sooner.” — Jim Highsmith"
  "“Programming isn't about what you know; it's about what you can figure out.” — Chris Pine"
  "“If debugging is the process of removing bugs, then programming must be the process of putting them in.” — Edsger Dijkstra"
  "“The most disastrous thing that you can ever learn is your first programming language.” — Alan Kay"
  "“The computer was born to solve problems that did not exist before.” — Bill Gates"
  "“Walking on water and developing software from a specification are easy if both are frozen.” — Edward V. Berard"
  "“I'm not a great programmer; I'm just a good programmer with great habits.” — Kent Beck"
  "“It’s not a bug — it’s an undocumented feature.” — Anonymous"
  "“A good programmer is someone who always looks both ways before crossing a one-way street.” — Doug Linder"
  "“Programs are meant to be read by humans and only incidentally for computers to execute.” — Donald Knuth"
  "“There are only two kinds of programming languages: those people always bitch about and those nobody uses.” — Bjarne Stroustrup"
  "“Measuring programming progress by lines of code is like measuring aircraft building progress by weight.” — Bill Gates"
  "“Software is like entropy: It is difficult to grasp, weighs nothing, and obeys the Second Law of Thermodynamics; i.e., it always increases.” — Norman Augustine"
  "“The function of good software is to make the complex appear to be simple.” — Grady Booch"
  "“There is no place like 127.0.0.1.” — Anonymous"
  "“The most important property of a program is whether it accomplishes the intention of its user.” — C.A.R. Hoare"
  "“Any sufficiently advanced technology is indistinguishable from magic.” — Arthur C. Clarke"
  "“To iterate is human, to recurse divine.” — L. Peter Deutsch"
  "“The best thing about a boolean is even if you are wrong, you are only off by a bit.” — Anonymous"
  "“Weeks of coding can save you hours of planning.” — Anonymous"
  "“Before software can be reusable it first has to be usable.” — Ralph Johnson"
  "“One man’s crappy software is another man’s full-time job.” — Jessica Gaston"
  "“Make it work, make it right, make it fast.” — Kent Beck"
  "“Computers are good at following instructions, but not at reading your mind.” — Donald Knuth"
  "“Good code is its own best documentation.” — Steve McConnell"
  "“It’s hardware that makes a machine fast. It’s software that makes a fast machine slow.” — Craig Bruce"
  "“The most important single aspect of software development is to be clear about what you are trying to build.” — Bjarne Stroustrup"
  "“If builders built buildings the way programmers wrote programs, then the first woodpecker that came along would destroy civilization.” — Gerald Weinberg"
  "“In software, the most beautiful code, the most beautiful functions, and the most beautiful programs are sometimes not there at all.” — Jon Bentley"
  "“Real programmers don't comment their code. If it was hard to write, it should be hard to understand.” — Anonymous"
  "“A primary cause of complexity is that software vendors uncritically adopt almost any feature that users want.” — Niklaus Wirth"
  "“Programs must be written for people to read, and only incidentally for machines to execute.” — Hal Abelson"
  "“The hardest part of programming is thinking about the problem.” — Steve McConnell"
  "“It always takes longer than you expect, even when you take into account Hofstadter's Law.” — Douglas Hofstadter"
  "“Premature optimization is the root of all evil.” — Donald Knuth"
  "“Computers are incredibly fast, accurate, and stupid; humans are incredibly slow, inaccurate, and brilliant; together they are powerful beyond imagination.” — Albert Einstein"
  "“The best performance improvement is the transition from the nonworking state to the working state.” — John Ousterhout"
  "“The key to performance is elegance, not battalions of special cases.” — Jon Bentley"
  "“Simple things should be simple, complex things should be possible.” — Alan Kay"
  "“Learning to program has no more to do with designing interactive software than learning to touch type has to do with writing poetry.” — Ted Nelson"
  "“A language that doesn’t affect the way you think about programming is not worth knowing.” — Alan Perlis"
  "“The trouble with programmers is that you can never tell what a programmer is doing until it’s too late.” — Seymour Cray"
  "“In theory, there is no difference between theory and practice. But, in practice, there is.” — Jan L.A. van de Snepscheut"
  "“Programming is not about typing, it's about thinking.” — Rich Hickey"
  "“The difference between theory and practice is that in theory, there is no difference between theory and practice.” — Richard Moore"
  "“Good judgment comes from experience, and experience comes from bad judgment.” — Fred Brooks"
  "“A bug is never just a mistake. It represents something bigger. An error of thinking that makes you who you are.” — Anonymous"
  "“If you think it's simple, then you have misunderstood the problem.” — Bjarne Stroustrup"
  "“Programming is the art of algorithm design and the craft of debugging errant code.” — Ellen Ullman"
  "“Sometimes it’s better to leave something alone, to pause, and that’s very true of programming.” — Joyce Wheeler"
  "“Good software, like good wine, takes time.” — Joel Spolsky"
  "“The only real mistake is the one from which we learn nothing.” — Henry Ford"
  "“Everything should be made as simple as possible, but not simpler.” — Albert Einstein"
  "“You can’t have great software without a great team, and most software teams behave like dysfunctional families.” — Jim McCarthy"
  "“The purpose of software engineering is to control complexity, not to create it.” — Pamela Zave"
  "“A good programmer is someone who always looks both ways before crossing a one-way street.” — Doug Linder"
  "“There is an easy way and a hard way. The hard part is finding the easy way.” — Anonymous"
  "“It's not that I'm so smart, it's just that I stay with problems longer.” — Albert Einstein"
  "“The most effective debugging tool is still careful thought, coupled with judiciously placed print statements.” — Brian Kernighan"
  "“The only way to learn a new programming language is by writing programs in it.” — Dennis Ritchie"
  "“The best error message is the one that never shows up.” — Thomas Fuchs"
  "“If you automate a mess, you get an automated mess.” — Rod Michael"
  "“A language that doesn't have everything is actually easier to program in than some that do.” — Dennis Ritchie"
  "“Good specifications will always improve programmer productivity far better than any programming tool or technique.” — Milt Bryce"
  "“The more code you have, the more places there are for bugs to hide.” — Anonymous"
  "“The only thing worse than starting something and failing... is not starting something.” — Seth Godin"
  "“You can’t trust code that you did not totally create yourself.” — Ken Thompson"
  "“Deleted code is debugged code.” — Jeff Sickel"
  "“The best thing about a boolean is even if you are wrong, you are only off by a bit.” — Anonymous"
  "“A good programmer is someone who looks both ways before crossing a one-way street.” — Doug Linder"
  "“The best way to predict the future is to implement it.” — David Heinemeier Hansson"
  "“People think that computer science is the art of geniuses, but the reality is the opposite, just many people doing things that build on each other, like a wall of mini stones.” — Donald Knuth"
  "“I don't care if it works on your machine! We are not shipping your machine!” — Vidiu Platon"
  "“There is no silver bullet in software development.” — Fred Brooks"
  "“Programming is the art of telling another human what one wants the computer to do.” — Donald Knuth"
  "“The best way to avoid failure is to fail constantly.” — Anonymous"
  "“Computers are like bikinis. They save people a lot of guesswork.” — Sam Ewing"
  "“If you think your users are idiots, only idiots will use it.” — Linus Torvalds"
  "“The fastest algorithm can frequently be replaced by one that is almost as fast and much easier to understand.” — Douglas Crockford"
  "“If you optimize everything, you will always be unhappy.” — Donald Knuth"
  "“Sometimes the elegant implementation is just a function. Not a method. Not a class. Not a framework. Just a function.” — John Carmack"
  "“Simplicity is prerequisite for reliability.” — Edsger W. Dijkstra"
  "“If you don't work on important problems, it's not likely that you'll do important work.” — Richard Hamming"
  "“All parts should go together without forcing. You must remember that the parts you are reassembling were disassembled by you. Therefore, if you can’t get them together again, there must be a reason. By all means, do not use a hammer.” — IBM Manual, 1925"
  "“To understand recursion, one must first understand recursion.” — Anonymous"
  "“Everything that can be automated will be automated.” — Robert Cannon"
  "“Programming is like sex: one mistake and you have to support it for the rest of your life.” — Michael Sinz"
)

# Pick a random quote
RANDOM_QUOTE=${QUOTES[$RANDOM % ${#QUOTES[@]}]}

# Get the username (compatible with all Unix-like systems)
USER_NAME=$(whoami)

# Check if tput is available, and fallback to 60 if not
if command -v tput &> /dev/null; then
    TTY_WIDTH=$(tput cols)
else
    TTY_WIDTH=60
fi

# Greet the fantastic person behind the screen
echo
echo -e "\033[0;36m💻 G\033[0;33ma\033[38;5;214ms\033[0;32mh \033[1;37mGash, Another SHell!\033[0m - Hello, \033[1;32m$USER_NAME!\033[0m"
echo -e "\033[1;97m💡\033[0m They said: \033[1;96m$RANDOM_QUOTE\033[0m" | fold -s -w $TTY_WIDTH
echo -e "\033[1;97m✨\033[0m \033[38;5;214mHave a nice day!\033[0m \033[1;97m😊\033[0m"
echo

# Unset variables to avoid polluting the environment
unset QUOTES RANDOM_QUOTE USER_NAME TTY_WIDTH