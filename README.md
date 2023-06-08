# LSP proxy between the native neovim LSP client and Isabelle

This proxy does some slight adjustment between the messages that Isabelle's LSP
component and neovim exchange (e.g. registering as a code action provider for
inserting proofs when using `try` or `sledgehammer`.

See `contrib/init.vim` for how I've integrated it in my vim setup. Note that not
all plugins might be present. The only reason it's not packaged as a neovim
plugin is that don't have the skills or time to do that - ideas and pull
requests are welcome!
