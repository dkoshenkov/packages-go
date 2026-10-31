// Optional browser check. Requires puppeteer-core and a local Chromium.
// PASSKEYS_DEMO_BINARY, PUPPETEER_MODULE and CHROME_PATH are supplied by the caller.
const assert = require('node:assert/strict');
const {spawn} = require('node:child_process');
const puppeteer = require(process.env.PUPPETEER_MODULE || 'puppeteer-core');
(async () => {
  const server = spawn(process.env.PASSKEYS_DEMO_BINARY, [], {stdio:['ignore','pipe','pipe']});
  let browser;
  try {
    await new Promise((resolve,reject) => {
      const timeout=setTimeout(()=>reject(new Error('server startup timeout')),10000);
      server.once('error',reject);
      server.once('exit',code=>reject(new Error(`server exited: ${code}`)));
      server.stderr.on('data',data=>{if(data.toString().includes('Experimental demo:')){clearTimeout(timeout);resolve();}});
    });
    browser = await puppeteer.launch({executablePath:process.env.CHROME_PATH,headless:true});
    const page=await browser.newPage();
    const cdp=await page.createCDPSession();
    await cdp.send('WebAuthn.enable');
    const makeAuthenticator=()=>cdp.send('WebAuthn.addVirtualAuthenticator',{options:{protocol:'ctap2',ctap2Version:'ctap2_1',transport:'internal',hasResidentKey:true,hasUserVerification:true,isUserVerified:true,automaticPresenceSimulation:true}});
    let {authenticatorId}=await makeAuthenticator();
    await page.goto('http://localhost:8080',{waitUntil:'networkidle0'});
    async function clickAndExpect(selector,text){
      await page.click(selector);
      await page.waitForFunction(expected=>document.querySelector('#status').textContent===expected,{timeout:15000},text);
      const me=await page.evaluate(()=>fetch('/me').then(r=>r.json()));
      assert.equal(me.authenticated,true);
    }
    await clickAndExpect('#register','Ключ сохранён. Вы вошли.');
    let credentials=await cdp.send('WebAuthn.getCredentials',{authenticatorId});
    assert.equal(credentials.credentials.length,1);
    const firstID=credentials.credentials[0].credentialId;
    await cdp.send('Network.clearBrowserCookies');
    await clickAndExpect('#login','Вы вошли.');
    // Add a distinct authenticator after a fresh login, preserving the account.
    await cdp.send('WebAuthn.removeVirtualAuthenticator',{authenticatorId});
    ({authenticatorId}=await makeAuthenticator());
    await clickAndExpect('#register','Ключ сохранён. Вы вошли.');
    credentials=await cdp.send('WebAuthn.getCredentials',{authenticatorId});
    assert.equal(credentials.credentials.length,1);
    assert.notEqual(credentials.credentials[0].credentialId,firstID);
    await cdp.send('Network.clearBrowserCookies');
    await clickAndExpect('#login','Вы вошли.');
    if(process.env.PASSKEYS_SCREENSHOT)await page.screenshot({path:process.env.PASSKEYS_SCREENSHOT});
    console.log(`PASS: ${await browser.version()}; register, discoverable login, second key, second-key login`);
  } finally {
    if(browser)await browser.close();
    server.kill('SIGTERM');
  }
})().catch(error=>{console.error(error);process.exitCode=1;});
