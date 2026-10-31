import base64, hashlib, json, pathlib, subprocess, tempfile
b64=lambda b:base64.urlsafe_b64encode(b).rstrip(b'=').decode()
def head(t,n):
    return bytes([(t<<5)|n]) if n<24 else bytes([(t<<5)|24,n]) if n<256 else bytes([(t<<5)|25])+n.to_bytes(2,'big')
def cbor(v):
    if isinstance(v,int):return head(0,v) if v>=0 else head(1,-1-v)
    if isinstance(v,bytes):return head(2,len(v))+v
    if isinstance(v,str):return head(3,len(v.encode()))+v.encode()
    if isinstance(v,dict):return head(5,len(v))+b''.join(cbor(k)+cbor(x) for k,x in v.items())
    raise TypeError(v)
with tempfile.TemporaryDirectory() as td:
    key=pathlib.Path(td)/'key.pem'
    subprocess.run(['openssl','ecparam','-name','prime256v1','-genkey','-noout','-out',str(key)],check=True)
    pub=subprocess.check_output(['openssl','pkey','-in',str(key),'-pubout','-outform','DER'])[-65:]
    cose=cbor({1:2,3:-7,-1:1,-2:pub[1:33],-3:pub[33:]})
    ident=b'independent-openssl-credential'; user=b'independent-user'
    challenge=bytes(range(32)); auth=hashlib.sha256(b'example.com').digest()+b'\x1d'+(2).to_bytes(4,'big')
    client=json.dumps({'origin':'https://example.com','challenge':b64(challenge),'type':'webauthn.get','crossOrigin':False},indent=2).encode()
    signed=pathlib.Path(td)/'signed'; signed.write_bytes(auth+hashlib.sha256(client).digest())
    sig=subprocess.check_output(['openssl','dgst','-sha256','-sign',str(key),str(signed)])
    regclient=json.dumps({'type':'webauthn.create','challenge':b64(challenge),'origin':'https://example.com'},separators=(',',':')).encode()
    regauth=auth[:32]+b'\x5d'+(1).to_bytes(4,'big')+bytes(16)+len(ident).to_bytes(2,'big')+ident+cose
    wrap=lambda response:{'id':b64(ident),'rawId':b64(ident),'type':'public-key','response':response,'clientExtensionResults':{}}
    fixture={'source':'Generated independently with Python stdlib CBOR encoding and OpenSSL ECDSA; no passkeys code used.', 'openssl':subprocess.check_output(['openssl','version']).decode().strip(),'challenge':b64(challenge),'userID':b64(user),'publicKey':b64(cose),'registration':wrap({'clientDataJSON':b64(regclient),'attestationObject':b64(cbor({'fmt':'none','attStmt':{},'authData':regauth}))}),'login':wrap({'clientDataJSON':b64(client),'authenticatorData':b64(auth),'signature':b64(sig),'userHandle':b64(user)})}
    pathlib.Path('passkeys/testdata/openssl-es256.json').write_text(json.dumps(fixture,indent=2)+'\n')
