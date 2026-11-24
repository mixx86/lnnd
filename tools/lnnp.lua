-- LNNP dissector for tshark/Wireshark. Field layout: docs/protocol.md.
-- Run it with `make sniff`.
local lnnp = Proto("lnnp", "LNN Protocol")

local types = {
    [0] = "HELLO", [1] = "CHALLENGE", [2] = "CHALLENGE_RESP", [3] = "ERROR",
    [4] = "LINK_SYM", [5] = "LINK_HARD", [6] = "SYMLINK_REQUEST",
    [7] = "HARDLINK_REQUEST", [8] = "OK", [9] = "RECOVER_REQUEST",
    [10] = "RESOLUTION",
}

local f = {
    magic   = ProtoField.bytes("lnnp.magic", "Magic"),
    ver     = ProtoField.uint8("lnnp.ver", "Version"),
    type    = ProtoField.uint8("lnnp.type", "Type", base.DEC, types),
    tag     = ProtoField.uint32("lnnp.tag", "Tag"),
    len     = ProtoField.uint32("lnnp.len", "Length"),
    uuid    = ProtoField.bytes("lnnp.uuid", "UUID"),
    pubkey  = ProtoField.bytes("lnnp.pubkey", "Public key"),
    nonce   = ProtoField.bytes("lnnp.nonce", "Nonce"),
    sig     = ProtoField.bytes("lnnp.sig", "Signature"),
    errcode = ProtoField.int64("lnnp.err.code", "Error code"),
    errmsg  = ProtoField.string("lnnp.err.msg", "Error message"),
    reqid   = ProtoField.uint32("lnnp.sender_req_id", "sender_req_id"),
    src     = ProtoField.string("lnnp.src", "src"),
    dst     = ProtoField.string("lnnp.dst", "dst"),
    acc     = ProtoField.bool("lnnp.accepted", "Accepted"),
    summary = ProtoField.string("lnnp.summary", "Summary"),
}
lnnp.fields = f

lnnp.prefs.port = Pref.uint("TCP port", 5771, "lnnd TCP_PORT")

local HEADER = 14

local function uuid_str(r)
    local h = tostring(r:bytes()):lower()
    return h:sub(1, 8) .. "-" .. h:sub(9, 12) .. "-" .. h:sub(13, 16) ..
        "-" .. h:sub(17, 20) .. "-" .. h:sub(21, 32)
end

local function short(r)
    local h = tostring(r:bytes()):lower()
    return h:sub(1, 8) .. "…"
end

local function pdu_len(tvb, pinfo, offset)
    return HEADER + tvb(offset + 10, 4):uint()
end

local function dissect_pdu(tvb, pinfo, tree)
    local t = tvb(5, 1):uint()
    local tag = tvb(6, 4):uint()
    local n = tvb(10, 4):uint()
    local name = types[t] or ("TYPE_" .. t)
    local sub = tree:add(lnnp, tvb(), "LNNP " .. name)
    sub:add(f.magic, tvb(0, 4))
    sub:add(f.ver, tvb(4, 1))
    sub:add(f.type, tvb(5, 1))
    sub:add(f.tag, tvb(6, 4))
    sub:add(f.len, tvb(10, 4))

    local s = string.format("%s tag=%d", name, tag)
    local p = HEADER
    if t == 0 and n == 48 then
        sub:add(f.uuid, tvb(p, 16)):append_text(" (" .. uuid_str(tvb(p, 16)) .. ")")
        sub:add(f.pubkey, tvb(p + 16, 32))
        s = s .. " uuid=" .. uuid_str(tvb(p, 16)) ..
            " key=" .. short(tvb(p + 16, 32))
    elseif t == 1 and n == 32 then
        sub:add(f.nonce, tvb(p, 32))
        s = s .. " nonce=" .. short(tvb(p, 32))
    elseif t == 2 and n == 64 then
        sub:add(f.sig, tvb(p, 64))
        s = s .. " sig=" .. short(tvb(p, 64))
    elseif t == 3 and n == 43 then
        sub:add(f.errcode, tvb(p, 8))
        local msg = tvb(p + 8, 35):stringz()
        sub:add(f.errmsg, tvb(p + 8, 35), msg)
        s = s .. string.format(" code=%s msg=%q",
            tostring(tvb(p, 8):int64()), msg)
    elseif t == 6 and n >= 6 then
        local id = tvb(p, 4):uint()
        sub:add(f.reqid, tvb(p, 4))
        s = s .. " sender_req_id=" .. id
        local sl = tvb(p + 4, 2):uint()
        if 6 + sl <= n then
            local src = tvb(p + 6, sl):string()
            sub:add(f.src, tvb(p + 6, sl), src)
            s = s .. string.format(" src=%q", src)
            local q = p + 6 + sl
            if q + 2 <= HEADER + n then
                local dl = tvb(q, 2):uint()
                if q + 2 + dl <= HEADER + n then
                    local dst = tvb(q + 2, dl):string()
                    sub:add(f.dst, tvb(q + 2, dl), dst)
                    s = s .. string.format(" dst=%q", dst)
                end
            end
        end
    elseif t == 10 and n == 5 then
        sub:add(f.reqid, tvb(p, 4))
        sub:add(f.acc, tvb(p + 4, 1))
        s = s .. string.format(" sender_req_id=%d accepted=%s",
            tvb(p, 4):uint(), tostring(tvb(p + 4, 1):uint() ~= 0))
    elseif n > 0 then
        s = s .. " len=" .. n
    end

    sub:add(f.summary, s):set_generated()
    pinfo.cols.protocol = "LNNP"
    local info = tostring(pinfo.cols.info)
    if info:find("^%[LNNP%]") then
        pinfo.cols.info:append(" | " .. s)
    else
        pinfo.cols.info = "[LNNP] " .. s
    end
    return tvb:len()
end

function lnnp.dissector(tvb, pinfo, tree)
    if tvb:len() < 4 or tvb(0, 4):uint() ~= 0x4C4E4E00 then
        return 0
    end
    dissect_tcp_pdus(tvb, tree, HEADER, pdu_len, dissect_pdu)
    return tvb:len()
end

local registered
function lnnp.init()
    local tcp = DissectorTable.get("tcp.port")
    if registered then
        tcp:remove(registered, lnnp)
    end
    registered = lnnp.prefs.port
    tcp:add(registered, lnnp)
end
