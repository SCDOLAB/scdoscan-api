/**
 * scdoscan.io Etherscan-level enhancement v2
 * Enhances: tx detail, homepage network stats, account/address detail
 */
(function(){
  const API = (location.hostname === 'localhost' || location.hostname === '127.0.0.1') ? '/api/v1' : 'https://api.scdoscan.io/api/v1';
  let currentTx = null, currentAddr = null;

  async function getJSON(url){ const r = await fetch(url); return r.json(); }
  function fmtSCDO(wei){ return (Number(wei)/1e9).toFixed(4) + ' SCDO'; }

  async function getTipHeight(){
    try { const j = await getJSON(API+'/blockcount'); return j.data || 0; } catch(e){ return 0; }
  }

  // ===== TX DETAIL =====
  async function enhanceTxDetail(){
    const m = location.search.match(/txhash=(0x[a-fA-F0-9]+)/) || location.hash.match(/txhash=(0x[a-fA-F0-9]+)/);
    if(!m) return;
    const txhash = m[1];
    if(currentTx === txhash) return;
    currentTx = txhash;

    const resp = await getJSON(API+'/tx?txhash='+txhash);
    if(!resp || resp.code !== 0 || !resp.data) return;
    const tx = resp.data;
    const receipt = tx.receipt || {};
    const status = receipt.failed ? '❌ Failed' : '✅ Success';
    const tipH = await getTipHeight();
    const confirms = tx.block ? (tipH - tx.block + 1) : 0;
    const gasUsed = receipt.usedGas || 0;
    const gasPrice = tx.gasprice || 0;
    const fee = tx.fee || 0;

    let logsHtml = '';
    if(receipt.logs){
      try {
        const logs = typeof receipt.logs === 'string' ? JSON.parse(receipt.logs) : receipt.logs;
        if(Array.isArray(logs) && logs.length){
          logsHtml = '<div style="margin-top:16px"><h4>📜 Event Logs ('+logs.length+')</h4>';
          logs.forEach((lg,i)=>{
            logsHtml += '<div style="background:#1a1a2e;padding:10px;margin:6px 0;border-radius:6px;font-size:12px">';
            logsHtml += '<div><b>Log #'+i+'</b> Address: <code>'+(lg.address||'-')+'</code></div>';
            logsHtml += '<div>Topics: <code>'+((lg.topics||[]).join(', '))+'</code></div>';
            logsHtml += '<div>Data: <code style="word-break:break-all">'+(lg.data||'-')+'</code></div>';
            logsHtml += '</div>';
          });
          logsHtml += '</div>';
        }
      } catch(e){}
    }

    let inputHtml = '';
    if(tx.payload && tx.payload !== '0x' && tx.payload.length > 10){
      const pd = tx.payload;
      let type = 'Contract Interaction';
      const sel = pd.substring(0,10);
      if(sel === '0xa9059cbb') type = 'ERC20: transfer(address,uint256)';
      else if(sel === '0x095ea7b3') type = 'ERC20: approve(address,uint256)';
      else if(sel === '0x23b872dd') type = 'ERC20: transferFrom(address,address,uint256)';
      else if(sel === '0x70a08231') type = 'ERC20: balanceOf(address)';
      else if(sel === '0x18160ddd') type = 'ERC20: totalSupply()';
      inputHtml = '<div style="margin-top:16px"><h4>📥 Input Data</h4><div style="background:#1a1a2e;padding:10px;border-radius:6px;font-size:12px">';
      inputHtml += '<div>Type: <b>'+type+'</b></div><div style="word-break:break-all"><code>'+pd+'</code></div></div></div>';
    }

    const panel = document.createElement('div');
    panel.id = 'scdo-enhanced-tx';
    panel.style.cssText = 'margin:16px 0;padding:16px;background:#16213e;border-radius:8px;color:#eee;font-family:monospace';
    panel.innerHTML =
      '<h3 style="margin-top:0">🔍 EVM Details</h3>'+
      '<table style="width:100%;font-size:13px;border-collapse:collapse">'+
      '<tr><td style="padding:4px;color:#888">Status</td><td style="padding:4px">'+status+'</td></tr>'+
      '<tr><td style="padding:4px;color:#888">Block</td><td style="padding:4px">#'+tx.block+' ('+confirms+' confirmations)</td></tr>'+
      '<tr><td style="padding:4px;color:#888">Nonce</td><td style="padding:4px">'+(tx.accountNonce||tx.nonce||'-')+'</td></tr>'+
      '<tr><td style="padding:4px;color:#888">Gas Used</td><td style="padding:4px">'+gasUsed.toLocaleString()+'</td></tr>'+
      '<tr><td style="padding:4px;color:#888">Gas Price</td><td style="padding:4px">'+(gasPrice/1e9).toFixed(4)+' Gwei</td></tr>'+
      '<tr><td style="padding:4px;color:#888">Fee</td><td style="padding:4px">'+fmtSCDO(fee)+'</td></tr>'+
      '<tr><td style="padding:4px;color:#888">Shard</td><td style="padding:4px">'+tx.shardnumber+'</td></tr>'+
      '</table>'+inputHtml+logsHtml;
    const old = document.getElementById('scdo-enhanced-tx'); if(old) old.remove();
    document.body.appendChild(panel);
  }

  // ===== ADDRESS PAGE =====
  async function enhanceAddress(){
    const m = location.search.match(/address=([0-9a-zA-Z]+)/);
    if(!m || !location.pathname.includes('/account/')) return;
    const addr = m[1];
    if(currentAddr === addr) return;
    currentAddr = addr;

    const resp = await getJSON(API+'/account?address='+addr);
    if(!resp || resp.code !== 0 || !resp.data) return;
    const acc = resp.data;

    const panel = document.createElement('div');
    panel.id = 'scdo-enhanced-addr';
    panel.style.cssText = 'margin:16px 0;padding:16px;background:#16213e;border-radius:8px;color:#eee;font-family:monospace';
    let tokensHtml = '';
    if(acc.src20Tokens && acc.src20Tokens.length){
      tokensHtml = '<div style="margin-top:12px"><h4>🪙 Token Holdings</h4>';
      acc.src20Tokens.forEach(t=>{
        tokensHtml += '<div style="padding:4px">'+(t.tokenName||t.symbol||'Token')+': '+(t.balance||0)+'</div>';
      });
      tokensHtml += '</div>';
    }
    panel.innerHTML =
      '<h3 style="margin-top:0">👤 Address Overview</h3>'+
      '<table style="width:100%;font-size:13px;border-collapse:collapse">'+
      '<tr><td style="padding:4px;color:#888">Balance</td><td style="padding:4px;font-size:16px;color:#53d8fb;font-weight:bold">'+fmtSCDO(acc.balance||0)+'</td></tr>'+
      '<tr><td style="padding:4px;color:#888">Transactions</td><td style="padding:4px">'+(acc.txcount||0)+'</td></tr>'+
      '<tr><td style="padding:4px;color:#888">Shard</td><td style="padding:4px">'+acc.shardnumber+'</td></tr>'+
      '<tr><td style="padding:4px;color:#888">Type</td><td style="padding:4px">'+(acc.accType===1?'📄 Contract':'👤 Wallet')+'</td></tr>'+
      '</table>'+tokensHtml;
    const old = document.getElementById('scdo-enhanced-addr'); if(old) old.remove();
    document.body.appendChild(panel);
  }

  // ===== HOMEPAGE =====
  async function enhanceHome(){
    if(location.pathname !== '/' && location.pathname !== '/index.html') return;
    if(document.getElementById('scdo-netstats')) return;
    try {
      const [blkR, gasR, pendingR] = await Promise.all([
        getJSON(API+'/blockcount'),
        getJSON(API+'/Avegas'),
        getJSON(API+'/pendingtxs?p=1&s=1&ps=1'),
      ]);
      const blockH = blkR.data || 0;
      const gas = gasR.data || {};
      const pending = (pendingR.data && pendingR.data.totalCount) || 0;
      const panel = document.createElement('div');
      panel.id = 'scdo-netstats';
      panel.style.cssText = 'max-width:1200px;margin:16px auto;padding:16px;background:linear-gradient(135deg,#16213e,#0f3460);border-radius:10px;color:#eee;display:flex;flex-wrap:wrap;gap:24px;justify-content:center;font-family:monospace';
      panel.innerHTML =
        '<div style="text-align:center"><div style="font-size:24px;font-weight:bold;color:#e94560">'+blockH.toLocaleString()+'</div><div style="color:#888;font-size:12px">Block Height</div></div>'+
        '<div style="text-align:center"><div style="font-size:24px;font-weight:bold;color:#53d8fb">'+pending+'</div><div style="color:#888;font-size:12px">Pending Txs</div></div>'+
        '<div style="text-align:center"><div style="font-size:24px;font-weight:bold;color:#53d8fb">'+((gas.avegas||0)/1e9).toFixed(4)+'</div><div style="color:#888;font-size:12px">Avg Gas (Gwei)</div></div>'+
        '<div style="text-align:center"><div style="font-size:24px;font-weight:bold;color:#53d8fb">'+((gas.highGasPrice||0)/1e9).toFixed(4)+'</div><div style="color:#888;font-size:12px">High Gas</div></div>'+
        '<div style="text-align:center"><div style="font-size:24px;font-weight:bold;color:#53d8fb">'+((gas.lowGasPrice||0)/1e9).toFixed(4)+'</div><div style="color:#888;font-size:12px">Low Gas</div></div>';
      const app = document.querySelector('#app') || document.body;
      app.insertBefore(panel, app.firstChild);
    } catch(e){}
  }

  // ===== BLOCK DETAIL =====
  async function enhanceBlock(){
    const m = location.search.match(/height=(\d+)/);
    if(!m || !location.pathname.includes('/block/')) return;
    const height = m[1];
    const shard = (location.search.match(/[?&]s=(\d+)/)||[])[1] || 1;
    if(document.getElementById('scdo-enhanced-block')) return;
    try {
      const resp = await getJSON(API+'/block?height='+height+'&shardnumber='+shard);
      if(!resp || resp.code !== 0 || !resp.data) return;
      const blk = resp.data;
      const panel = document.createElement('div');
      panel.id = 'scdo-enhanced-block';
      panel.style.cssText = 'margin:16px 0;padding:16px;background:#16213e;border-radius:8px;color:#eee;font-family:monospace';
      panel.innerHTML =
        '<h3 style="margin-top:0">⛓ Block Details</h3>'+
        '<table style="width:100%;font-size:13px;border-collapse:collapse">'+
        '<tr><td style="padding:4px;color:#888">Height</td><td>#'+blk.height+'</td></tr>'+
        '<tr><td style="padding:4px;color:#888">Hash</td><td style="word-break:break-all"><code>'+blk.headHash+'</code></td></tr>'+
        '<tr><td style="padding:4px;color:#888">Parent Hash</td><td style="word-break:break-all"><code>'+blk.preBlockHash+'</code></td></tr>'+
        '<tr><td style="padding:4px;color:#888">Miner</td><td><code>'+blk.miner+'</code></td></tr>'+
        '<tr><td style="padding:4px;color:#888">Tx Count</td><td>'+blk.txcount+'</td></tr>'+
        '<tr><td style="padding:4px;color:#888">Difficulty</td><td>'+(blk.difficulty||'-')+'</td></tr>'+
        '<tr><td style="padding:4px;color:#888">Shard</td><td>'+blk.shardnumber+'</td></tr>'+
        '</table>';
      document.body.appendChild(panel);
    } catch(e){}
  }

  // ===== CONTRACT PAGE =====
  async function enhanceContract(){
    const m = location.search.match(/address=([0-9a-zA-Z]+)/);
    if(!m || !location.pathname.includes('/contract/')) return;
    const addr = m[1];
    if(document.getElementById('scdo-enhanced-contract')) return;
    try {
      const resp = await getJSON(API+'/contract?address='+addr);
      if(!resp || resp.code !== 0 || !resp.data) return;
      const c = resp.data;
      const panel = document.createElement('div');
      panel.id = 'scdo-enhanced-contract';
      panel.style.cssText = 'margin:16px 0;padding:16px;background:#16213e;border-radius:8px;color:#eee;font-family:monospace';
      const bc = c.contractCreationCode || '';
      panel.innerHTML =
        '<h3 style="margin-top:0">📄 Contract Info</h3>'+
        '<table style="width:100%;font-size:13px;border-collapse:collapse">'+
        '<tr><td style="padding:4px;color:#888">Address</td><td><code>'+c.address+'</code></td></tr>'+
        '<tr><td style="padding:4px;color:#888">Balance</td><td>'+fmtSCDO(c.balance||0)+'</td></tr>'+
        '<tr><td style="padding:4px;color:#888">Shard</td><td>'+c.shardNumber+'</td></tr>'+
        '</table>'+
        (bc ? '<div style="margin-top:12px"><h4>Bytecode ('+bc.length+' chars)</h4><div style="background:#1a1a2e;padding:8px;border-radius:4px;max-height:150px;overflow:auto;font-size:11px;word-break:break-all"><code>'+bc+'</code></div></div>' : '');
      document.body.appendChild(panel);
    } catch(e){}
  }

  // ===== SEARCH AUTOCOMPLETE =====
  function initSearch(){
    if(document.getElementById('scdo-search-suggest')) return;
    const input = document.querySelector('input[placeholder*="address"], input[placeholder*="hash"], input[placeholder*="block"], input[type="text"]');
    if(!input) return;
    input.setAttribute('autocomplete', 'off');

    const sugg = document.createElement('div');
    sugg.id = 'scdo-search-suggest';
    sugg.style.cssText = 'position:absolute;z-index:9999;background:#16213e;border:1px solid #0f3460;border-radius:6px;max-height:300px;overflow-y:auto;display:none;min-width:300px;box-shadow:0 4px 12px rgba(0,0,0,0.5)';
    input.parentElement.style.position = 'relative';
    input.parentElement.appendChild(sugg);

    let timer;
    input.addEventListener('input', function(){
      clearTimeout(timer);
      const q = input.value.trim();
      if(q.length < 3){ sugg.style.display='none'; return; }
      timer = setTimeout(async ()=>{
        try {
          const j = await getJSON(API+'/search?content='+encodeURIComponent(q));
          if(j.code === 0 && j.data){
            const d = j.data;
            sugg.innerHTML = '';
            if(d.block){
              sugg.innerHTML += '<div style="padding:8px 12px;cursor:pointer;color:#53d8fb" onclick="location.href=\'/block/detail?height='+d.block.height+'&s='+d.block.shardnumber+'\'">⛓ Block #'+d.block.height+' ('+d.block.txcount+' txs)</div>';
            }
            if(d.account){
              sugg.innerHTML += '<div style="padding:8px 12px;cursor:pointer;color:#eee" onclick="location.href=\'/account/detail?address='+d.account.address+'\'">👤 '+d.account.address.substring(0,20)+'... ('+d.account.txcount+' txs)</div>';
            }
            if(d.tx){
              sugg.innerHTML += '<div style="padding:8px 12px;cursor:pointer;color:#e94560" onclick="location.href=\'/transaction/detail?txhash='+d.tx.hash+'\'">📝 '+d.tx.hash.substring(0,20)+'...</div>';
            }
            sugg.style.display = sugg.innerHTML ? 'block' : 'none';
          }
        } catch(e){}
      }, 300);
    });
    document.addEventListener('click', function(e){
      if(!sugg.contains(e.target) && e.target !== input) sugg.style.display='none';
    });
  }

  // ===== FLOATING STATUS BAR =====
  async function initStatusBar(){
    if(document.getElementById('scdo-statusbar')) return;
    try {
      const [blkR, pendingR] = await Promise.all([
        getJSON(API+'/blockcount'),
        getJSON(API+'/pendingtxs?p=1&s=1&ps=1'),
      ]);
      const h = blkR.data || 0;
      const p = (pendingR.data && pendingR.data.totalCount) || 0;
      const bar = document.createElement('div');
      bar.id = 'scdo-statusbar';
      bar.style.cssText = 'position:fixed;bottom:0;left:0;right:0;background:#0f3460;color:#aaa;font-size:11px;padding:3px 16px;z-index:9998;display:flex;justify-content:space-between;font-family:monospace';
      bar.innerHTML = '<span>SCDO Chain | Height: '+h.toLocaleString()+'</span><span>Pending: '+p+' | scdoscan.io enhanced</span>';
      document.body.appendChild(bar);
    } catch(e){}
  }

  // Watch routes
  let lastUrl = '';
  setInterval(()=>{
    const cur = location.pathname + location.search;
    if(cur !== lastUrl){
      lastUrl = cur;
      currentTx = null; currentAddr = null;
      setTimeout(enhanceTxDetail, 1000);
      setTimeout(enhanceAddress, 1200);
      setTimeout(enhanceBlock, 1300);
      setTimeout(enhanceContract, 1400);
      setTimeout(enhanceHome, 1500);
      setTimeout(initSearch, 2000);
    }
  }, 500);
  setTimeout(enhanceHome, 2000);
  setTimeout(initSearch, 3000);
  setTimeout(initStatusBar, 4000);
})();
