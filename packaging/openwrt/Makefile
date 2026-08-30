include $(TOPDIR)/rules.mk

PKG_NAME:=bagualu
PKG_RELEASE?=12
PKG_VERSION?=0.1.0
BAGUALU_BINARY:=$(abspath $(dir $(lastword $(MAKEFILE_LIST)))/bagualu)
PKG_BUILD_DIR:=$(BUILD_DIR)/$(PKG_NAME)-$(PKG_VERSION)

include $(INCLUDE_DIR)/package.mk

define Package/bagualu
  SECTION:=net
  CATEGORY:=Network
  TITLE:=八卦炉代理池
  DEPENDS:=+luci-base +rpcd
endef

define Package/bagualu/conffiles
/etc/config/bagualu
endef

define Build/Prepare
	mkdir -p $(PKG_BUILD_DIR)
	test -x "$(BAGUALU_BINARY)" || { echo "missing executable $(BAGUALU_BINARY)" >&2; exit 1; }
	$(CP) "$(BAGUALU_BINARY)" $(PKG_BUILD_DIR)/bagualu
endef

define Build/Compile
	chmod 0755 $(PKG_BUILD_DIR)/bagualu
endef

define Package/bagualu/install
	$(INSTALL_DIR) $(1)/usr/bin $(1)/etc/init.d $(1)/etc/config
	$(INSTALL_DIR) $(1)/usr/share/luci/menu.d $(1)/usr/share/rpcd/acl.d
	$(INSTALL_DIR) $(1)/usr/lib/lua/luci/controller $(1)/usr/lib/lua/luci/view/bagualu $(1)/usr/lib/lua/luci/model/cbi/bagualu
	$(INSTALL_BIN) $(PKG_BUILD_DIR)/bagualu $(1)/usr/bin/bagualu
	$(INSTALL_BIN) ./etc/init.d/bagualu $(1)/etc/init.d/bagualu
	$(INSTALL_CONF) ./etc/config/bagualu $(1)/etc/config/bagualu
	$(INSTALL_DATA) ./usr/share/luci/menu.d/luci-app-bagualu.json $(1)/usr/share/luci/menu.d/
	$(INSTALL_DATA) ./usr/share/rpcd/acl.d/luci-app-bagualu.json $(1)/usr/share/rpcd/acl.d/
	$(INSTALL_DATA) ./usr/lib/lua/luci/controller/bagualu.lua $(1)/usr/lib/lua/luci/controller/
	$(INSTALL_DATA) ./usr/lib/lua/luci/view/bagualu/status.htm $(1)/usr/lib/lua/luci/view/bagualu/
	$(INSTALL_DATA) ./usr/lib/lua/luci/view/bagualu/control.htm $(1)/usr/lib/lua/luci/view/bagualu/
	$(INSTALL_DATA) ./usr/lib/lua/luci/model/cbi/bagualu/config.lua $(1)/usr/lib/lua/luci/model/cbi/bagualu/
endef

$(eval $(call BuildPackage,bagualu))
