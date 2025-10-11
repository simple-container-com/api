# 🛡️ Simple Container AI Assistant - Security Boundaries

## 🚨 **CRITICAL SECURITY WARNING**

**The Simple Container AI Assistant's credential obfuscation system does NOT protect against ALL file access methods.** 

There are **UNSAFE** file access methods that completely bypass our security and expose raw credentials to the LLM.

---

## ✅ **SAFE - Protected File Access**

These commands apply comprehensive credential obfuscation before exposing content to the LLM:

### **Simple Container Chat Commands** 
- **✅ `/file <filename>`** - Protected file reading with full obfuscation
- **✅ `/config`** - Protected configuration display 
- **✅ `/show <stack>`** - Protected stack configuration display

### **What Gets Protected:**
- 🔒 **Private keys** (GCP, AWS, SSH) → `••••••••••••••••••••••••••••••••`
- 🔒 **API tokens** (OpenAI, GitHub, etc.) → `sk-•••••••••••••••••••••••••••••••••••••••••••••••••••`
- 🔒 **Database URIs** with credentials → `mongodb://••••••••:••••••••@host`
- 🔒 **All values in `values` section** → Obfuscated regardless of key name
- 🔒 **Embedded JSON/YAML credentials** → Parsed and obfuscated recursively

---

## ❌ **UNSAFE - Unprotected File Access**

These methods **BYPASS ALL SECURITY** and expose raw credentials:

### **Cascade Native Tools**
- **❌ `> read filename`** - Direct file access, NO OBFUSCATION
- **❌ `> cat filename`** - Direct file access, NO OBFUSCATION

### **IDE File Operations**
- **❌ File preview/hover** - Direct IDE access, NO OBFUSCATION
- **❌ File explorer clicking** - Direct IDE access, NO OBFUSCATION  
- **❌ Search results** - Content visible in search, NO OBFUSCATION

### **Manual Operations**
- **❌ Copy-paste from editor** - Manual access, NO OBFUSCATION
- **❌ Terminal `cat` commands** - Shell access, NO OBFUSCATION
- **❌ Text editor opening files** - Direct editor access, NO OBFUSCATION

---

## 🎯 **Real-World Example**

### **❌ DANGEROUS - Exposes Real Credentials**
```bash
> read .sc/stacks/dist/secrets.yaml
```
**Result**: Raw GCP private keys, API tokens, and all secrets exposed to LLM:
```json
"private_key": "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDBECmFTP0AcGAm..."
```

### **✅ SAFE - Protects All Credentials**  
```bash
/file .sc/stacks/dist/secrets.yaml
```
**Result**: All credentials properly obfuscated:
```json
"private_key": "••••••••••••••••••••••••••••••••"
```

---

## 🔒 **Security Architecture**

### **How Protection Works**
```
File Reading Security Matrix:
├── Simple Container Commands (PROTECTED) ✅
│   ├── /file → handleReadProjectFile() → obfuscateCredentials()
│   ├── /config → handleConfig() → obfuscateCredentials()  
│   └── /show → handleShowStack() → obfuscateCredentials()
│
└── All Other Access Methods (UNPROTECTED) ❌
    ├── Cascade native tools → os.ReadFile() → RAW CONTENT
    ├── IDE file operations → Direct access → RAW CONTENT
    └── Manual operations → User access → RAW CONTENT
```

### **Why This Limitation Exists**
- **Our protection is application-level** - We can only secure our own code paths
- **Cascade runs independently** - Native tools bypass our application entirely
- **IDE operations are external** - File preview/editing happens outside our control
- **System-level protection** would require OS-level file system interception

---

## 🚨 **Critical Risk Scenarios**

### **Accidental Exposure**
- User types `> read secrets.yaml` instead of `/file secrets.yaml`
- **Result**: Real private keys sent to LLM processing

### **IDE Integration**
- User hovers over secrets file in file explorer
- **Result**: Raw credentials visible in preview popup

### **Copy-Paste Operations**  
- User copies content from IDE and pastes in chat
- **Result**: Real credentials sent to LLM processing

### **Search Operations**
- User searches for text within secrets files
- **Result**: Credentials visible in search results

---

## 🛠️ **Recommended Security Practices**

### **✅ DO - Safe Practices**
1. **Always use Simple Container commands** for viewing config files
2. **Use `/file` instead of `> read`** for all file viewing
3. **Use `/config` for configuration analysis** 
4. **Use `/show <stack>` for stack inspection**
5. **Train team members** on protected commands

### **❌ DON'T - Dangerous Practices**  
1. **Don't use Cascade native file tools** on secrets
2. **Don't preview secrets files** in IDE
3. **Don't copy-paste raw file content** into chat
4. **Don't use terminal commands** to view secrets in chat context

### **🎯 Quick Reference Card**
```
SAFE:   /file secrets.yaml    ✅ Protected
UNSAFE: > read secrets.yaml   ❌ Raw exposure

SAFE:   /config              ✅ Protected  
UNSAFE: Copy-paste from IDE  ❌ Raw exposure

SAFE:   /show stack-name     ✅ Protected
UNSAFE: File hover preview   ❌ Raw exposure
```

---

## 🔮 **Future Security Enhancements**

### **Planned Improvements**
1. **System-level file interception** - Intercept all file access at OS level
2. **IDE plugin integration** - Extend protection to IDE operations
3. **Cascade plugin hooks** - Integrate with Cascade's native tools if possible
4. **User warnings** - Detect unsafe file access attempts and warn users

### **Current Status**
- **✅ Application-level protection** - Complete for Simple Container commands
- **❌ System-level protection** - Not yet implemented
- **❌ IDE integration** - Not yet available  
- **❌ Cascade integration** - Architecture limitation

---

## 🆘 **If Credentials Were Exposed**

If you accidentally used unsafe file access and exposed credentials:

### **Immediate Actions**
1. **Rotate exposed credentials** immediately
2. **Check LLM conversation history** for credential exposure  
3. **Update secrets.yaml** with new credentials
4. **Review team practices** to prevent recurrence

### **Credential Rotation Guides**
- **GCP Service Accounts**: Generate new key, update secrets.yaml
- **API Tokens**: Revoke old token, generate new one
- **Database Passwords**: Change password, update connection strings

---

## 📞 **Questions or Issues**

If you have questions about security boundaries or need help with secure file access:

1. **Use protected commands only** - `/file`, `/config`, `/show`
2. **Report security concerns** to the development team
3. **Train team members** on these security boundaries
4. **Follow the safe practices** outlined in this document

---

**⚠️ Remember: Your security is only as strong as your least secure access method. Always use protected Simple Container commands for viewing credentials.**
