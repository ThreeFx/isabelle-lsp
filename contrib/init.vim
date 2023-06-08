lua<<EOF
  local lspconfig = require 'lspconfig'
  local configs = require 'lspconfig.configs'

  -- We use this local function as a helper in conjunction
  -- with the optional diagnostics passed to code actions
  -- to specify the range to be replaced
  isabelle_get_diagnostics = function()
    local _, l = unpack(vim.fn.getcurpos())
    local line = vim.api.nvim_get_current_line()
    local startc = 0
    local endc = 0
    if string.find(line, "try0? *") then
      startc, endc = string.find(line, "try0? *")
      startc = startc - 1
    elseif string.find(line, "sledgehammer *") then
      startc, endc = string.find(line, "sledgehammer *")
      startc = startc - 1
    end
    if string.find(line, "sorry") then
      _, endc = string.find(line, "sorry")
    end

    return {
      {
        range = {
          start = {line = l, character = startc},
          ['end'] = {line = l, character = endc},
        },
        message = 'unfinished proof',
      },
    }
  end

  local isabelle_dynamic_output = ''
  function isabelle_display_dynamic_output()
    lines = {}
    for line in string.gmatch(isabelle_dynamic_output,'[^\r\n]+') do
      if line ~= '' then
        table.insert(lines, line)
      end
    end
    if #lines == 0 then
      return
    end
    return vim.lsp.util.open_floating_preview(lines, 'plaintext', nil)
  end

  configs.isabelle = {
    default_config = {
      cmd = {
        '/Users/bfiedler/Projects/IsabelleLSP/isabelle-lsp',
        'vscode_server',
        '-o',
        'vscode_pide_extensions',
        '-v',
        '-m',
        'ascii',
        '-L',
        '/tmp/loggggg',
      },
      filetypes = {'isabelle'},
      root_dir = function(fname)
        return lspconfig.util.find_git_ancestor(fname) or vim.loop.os_homedir()
      end,
      handlers = {
        ['PIDE/decoration'] = function(err, params, ctx, config)
          -- Use some string manipulation to format the highlight group
          local ty = params['type']
          ty = string.gsub(ty, '^.', string.upper)
          ty = string.gsub(ty, '_.', string.upper)
          ty = string.gsub(ty, '_', '')
          local hlg = 'IsaDecoration' .. ty

          -- Look up the namespace (bad function naming, I know...)
          local nsn = ty
          local ns = vim.api.nvim_create_namespace(nsn)

          -- TODO: can we get the bufnr from params.uri?
          local bnr = 0

          -- TODO: this might be inefficient
          vim.api.nvim_buf_clear_namespace(0, ns, 0, -1)
          for _,r in ipairs(params.content) do
            local sl, sc, el, ec = unpack(r.range)
            if sl == el then
              vim.api.nvim_buf_add_highlight(bnr, ns, hlg, sl, sc, ec)
            elseif el == sl + 1 then
              vim.api.nvim_buf_add_highlight(bnr, ns, hlg, sl, sc, -1)
              vim.api.nvim_buf_add_highlight(bnr, ns, hlg, el, 0, ec)
            else
              vim.api.nvim_buf_add_highlight(bnr, ns, hlg, sl, sc, -1)
              for l=sl+1,el-1 do
                vim.api.nvim_buf_add_highlight(bnr, ns, hlg, l, 0, -1)
              end
              vim.api.nvim_buf_add_highlight(bnr, ns, hlg, el, 0, ec)
            end
          end
        end,
        ['PIDE/dynamic_output'] = function(err, params, ctx, config)
          isabelle_dynamic_output = params.content
        end,
      },
      settings = {},
      on_attach = function(client, bufnr)
        vim.api.nvim_create_autocmd({"CursorMoved","CursorMovedI"}, {
          buffer = bufnr,
          callback = function(info)
            local b = info.buf
            local bwn = vim.fn.bufwinnr(b)
            local cp = vim.fn.getcurpos(bwn)
            local uri = 'file://' .. vim.fn.expand('%:p')

            client.notify('PIDE/caret_update', {
              uri = uri,
              line = cp[2]-1,
              character = cp[3]-1,
            })
          end,
        })
      end,
    },
  }
EOF
