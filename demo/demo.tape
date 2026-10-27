# Record with: vhs demo/demo.tape   (run from the repository root)
# Everything shown comes from demo/setup.sh's generated workspace.
# Afterwards: hdiutil detach /tmp/nodekill-demo/home; rm -rf /tmp/nodekill-demo
Output demo/demo.gif
Set Shell bash
Set FontSize 15
Set Width 1280
Set Height 580
Set Padding 16
Set TypingSpeed 50ms
Set Framerate 20

Hide
Type "source demo/setup.sh && clear" Enter
Sleep 40s
Type "clear" Enter
Sleep 500ms
Show

# Projects, caches, npx, global packages and Node versions, each with its level.
Type "nodekill --dry-run" Sleep 400ms Enter
Sleep 4s
Down Sleep 800ms
Down Sleep 800ms
# s: the safe folders in idle projects.
Type "s" Sleep 1.5s
# tab: group by framework, then by idle time.
Tab Sleep 2s
Tab Sleep 2s
Tab Sleep 400ms
# The machine-wide part: caches, npx, globals, Node versions.
Type "G" Sleep 2.5s
# a: every safe item, then the confirmation and a dry run.
Type "a" Sleep 1.2s
Enter Sleep 3.5s
Type "y" Sleep 2.5s
Type "q" Sleep 3s
