{
  description = "Builds Seedhammer disk image for Raspberry Pi";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/23.11";
    nixpkgs-unstable.url = "github:NixOS/nixpkgs/master";
    utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, nixpkgs-unstable, utils }:
    utils.lib.eachDefaultSystem (system:
      let
        arch = builtins.head (builtins.split "-" system);
        localpkgs = import nixpkgs {
          inherit system;
        };
        localpkgs-unstable = import nixpkgs-unstable {
          inherit system;
        };
        crosspkgs = import nixpkgs {
          inherit system;
          crossSystem = {
            config = "armv6l-unknown-linux-musleabihf";
            gcc = {
              arch = "armv6k";
              fpu = "vfp";
            };
          };
        };
        crosspkgs-unstable = import nixpkgs-unstable {
          inherit system;
          crossSystem = {
            config = "armv6l-unknown-linux-musleabihf";
            gcc = {
              arch = "armv6k";
              fpu = "vfp";
            };
          };
        };
        timestamp = "2009/01/03T12:15:05";
        loader-lib = "ld-musl-armhf.so.1";
      in
      {
        formatter = localpkgs.nixpkgs-fmt;
        lib = {
          mkkernel =
            let
              pkgs = crosspkgs;
              panel-firmware = self.lib.${system}.panel-firmware;
            in
            debug: pkgs.stdenv.mkDerivation {
              name = "Raspberry Pi Linux kernel";

              src = pkgs.fetchFromGitHub {
                owner = "raspberrypi";
                repo = "linux";
                rev = "3bb5880ab3dd31f75c07c3c33bf29c5d469b28f3";
                hash = "sha256-v4ennISbEk0ApnfDRZKCJOHfO8qLdlBNlGjffkOy7LY=";
                # Remove files that introduce case sensitivity clashes on darwin.
                postFetch = ''
                  rm $out/include/uapi/linux/netfilter/xt_*.h
                  rm $out/include/uapi/linux/netfilter_ipv4/ipt_*.h
                  rm $out/include/uapi/linux/netfilter_ipv6/ip6t_*.h
                  rm $out/net/netfilter/xt_*.c
                  rm $out/tools/memory-model/litmus-tests/Z6.0+poonce*
                '';
              };

              # For reproducible builds.
              KBUILD_BUILD_TIMESTAMP = timestamp;
              KBUILD_BUILD_USER = "seedhammer";
              KBUILD_BUILD_HOST = "seedhammer.com";

              enableParallelBuilding = true;

              makeFlags = [
                "ARCH=arm"
                "CROSS_COMPILE=${pkgs.stdenv.cc.targetPrefix}"
              ];

              depsBuildBuild = [ pkgs.buildPackages.stdenv.cc ];

              nativeBuildInputs = with pkgs.buildPackages; [
                elf-header
                bison
                flex
                openssl
                bc
                perl
                # Include getty package
                getty
              ];

              patches = [
                ./patches/kernel_missing_includes.patch
              ];

              hardeningDisable = [ "bindnow" "format" "fortify" "stackprotector" "pic" "pie" ];

              postPatch = ''
                patchShebangs scripts/config
              '';

              configurePhase = ''
                export HOSTCC=$CC_FOR_BUILD
                export HOSTCXX=$CXX_FOR_BUILD
                export HOSTAR=$AR_FOR_BUILD
                export HOSTLD=$LD_FOR_BUILD

                make $makeFlags -j$NIX_BUILD_CORES \
                  HOSTCC=$HOSTCC HOSTCXX=$HOSTCXX HOSTAR=$HOSTAR HOSTLD=$HOSTLD \
                  CC=$CC OBJCOPY=$OBJCOPY OBJDUMP=$OBJDUMP READELF=$READELF \
                  HOSTCFLAGS="-D_POSIX_C_SOURCE=200809L" \
                  bcmrpi_defconfig

                ./scripts/config --set-str EXTRA_FIRMWARE panel.bin
                ./scripts/config --set-str EXTRA_FIRMWARE_DIR ${panel-firmware}
                # Disable networking (including bluetooth).
                ./scripts/config --disable NET
                ./scripts/config --disable INET
                ./scripts/config --disable NETFILTER
                ./scripts/config --disable PROC_SYSCTL
                ./scripts/config --disable FSCACHE
                # There's no need for security models, and leaving it enabled
                # leads to build errors because of the files removed in postFetch above.
                ./scripts/config --disable SECURITY
                # Disable sound support.
                ./scripts/config --disable SOUND
                # Disable features we don't need.
                ./scripts/config --disable EXT4_FS
                ./scripts/config --disable F2FS_FS
                ./scripts/config --disable PSTORE
                ./scripts/config --disable INPUT_TOUCHSCREEN
                ./scripts/config --disable RC_MAP
                ./scripts/config --disable NAMESPACES
                ./scripts/config --disable INPUT
                # Enable v4l2
                ./scripts/config --enable MEDIA_SUPPORT
                ./scripts/config --enable VIDEO_V4L2
                ./scripts/config --enable VIDEO_DEV
                # Enable camera driver.
                ./scripts/config --enable I2C_BCM2835
                ./scripts/config --enable I2C_MUX
                ./scripts/config --enable REGULATOR_FIXED_VOLTAGE
                ./scripts/config --enable I2C_MUX_PINCTRL
                ./scripts/config --enable VIDEO_BCM2835_UNICAM
                ./scripts/config --enable VIDEO_CODEC_BCM2835
                ./scripts/config --enable VIDEO_ISP_BCM2835
                # Raspberry camera module 1.
                ./scripts/config --enable VIDEO_OV5647
                # Raspberry camera module 3.
                ./scripts/config --enable VIDEO_IMX708
                ./scripts/config --enable VIDEO_DW9807_VCM
                # Enable SPI.
                ./scripts/config --enable SPI_BCM2835
                # Enable FTDI USB serial driver.
                ./scripts/config --enable USB_SERIAL
                ./scripts/config --enable USB_SERIAL_FTDI_SIO
                # Disable HDMI framebuffer device.
                ./scripts/config --disable FB_BCM2708
                # Enable display driver.
                ./scripts/config --enable BACKLIGHT_GPIO
                ./scripts/config --enable DRM
                ./scripts/config --enable DRM_PANEL_MIPI_DBI
                ./scripts/config --disable LOGO
                ./scripts/config --enable FRAMEBUFFER_CONSOLE_DEFERRED_TAKEOVER
                # Enable slower but better supported USB driver.
                ./scripts/config --disable USB_DWCOTG
                ./scripts/config --enable USB_DWC2

                # For Raspberry Pi Zero 2.
                ./scripts/config --enable ARCH_MULTI_V7
                ./scripts/config --enable ARM_ERRATA_643719
                # Enabling VDSO for some reason introduces enough differences between
                # Linux and macOS that the resulting kernel image differs.
                ./scripts/config --disable VDSO
              '' + (if debug then ''
                ./scripts/config --enable USB_G_SERIAL
              '' else "");
              
              # other phases and configurations
            };
          # other definitions
        };
        # other configurations
      });
}