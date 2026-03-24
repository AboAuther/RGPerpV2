// SPDX-License-Identifier: MIT
pragma solidity ^0.8.26;

import "../src/MockUSDC.sol";
import "../src/Vault.sol";
import "../src/DepositRouterFactory.sol";
import "./TestBase.sol";

contract VaultTest is TestBase {
    MockUSDC internal token;
    Vault internal vault;
    DepositRouterFactory internal factory;

    address internal admin = address(0xA11CE);
    address internal executor = address(0xB0B);
    address internal user = address(0xCAFE);

    function setUp() public {
        token = new MockUSDC();
        vault = new Vault(admin);
        factory = new DepositRouterFactory(admin, address(vault), address(token));

        vm.prank(admin);
        vault.setTokenAllowed(address(token), true);
        vm.prank(admin);
        vault.setWithdrawExecutor(executor, true);
    }

    function testFactoryPredictAndCreateRouter() public {
        bytes32 salt = keccak256("user-1");
        address predicted = factory.predictRouter(1, salt);

        vm.prank(admin);
        address router = factory.createRouter(1, salt);
        assertEq(router, predicted, "router address mismatch");
    }

    function testRouterForwardDepositToVault() public {
        bytes32 salt = keccak256("user-2");
        vm.prank(admin);
        address payable router = payable(factory.createRouter(2, salt));

        token.mint(router, 100e6);
        DepositRouter(router).forward();

        assertEq(token.balanceOf(address(vault)), 100e6, "vault should receive forwarded tokens");
    }

    function testOnlyExecutorCanWithdraw() public {
        token.mint(address(vault), 50e6);

        vm.expectRevert(bytes("NOT_EXECUTOR"));
        vault.withdraw(address(token), user, 10e6, bytes32("wd-unauthorized"));

        vm.prank(executor);
        vault.withdraw(address(token), user, 10e6, bytes32("wd-authorized"));
        assertEq(token.balanceOf(user), 10e6, "user should receive withdrawn amount");
    }

    function testWithdrawRejectsReplayByWithdrawId() public {
        token.mint(address(vault), 50e6);

        vm.prank(executor);
        vault.withdraw(address(token), user, 10e6, bytes32("wd-replay"));

        vm.prank(executor);
        vm.expectRevert(bytes("WITHDRAW_ALREADY_PROCESSED"));
        vault.withdraw(address(token), user, 10e6, bytes32("wd-replay"));
    }

    function testPauseBlocksWithdraw() public {
        token.mint(address(vault), 50e6);

        vm.prank(admin);
        vault.pause();

        vm.prank(executor);
        vm.expectRevert(bytes("PAUSED"));
        vault.withdraw(address(token), user, 1e6, bytes32("wd-paused"));
    }

    function testRescueTokenBlocksAllowedCustodyToken() public {
        token.mint(address(vault), 10e6);

        vm.prank(admin);
        vm.expectRevert(bytes("TOKEN_RESCUE_BLOCKED"));
        vault.rescueToken(address(token), user, 1e6, bytes32("rescue-usdc"));
    }

    function testRouterSweepNativeRequiresOwner() public {
        bytes32 salt = keccak256("user-3");
        vm.prank(admin);
        address payable router = payable(factory.createRouter(3, salt));

        vm.deal(router, 1 ether);

        vm.prank(user);
        vm.expectRevert(bytes("NOT_OWNER"));
        DepositRouter(router).sweepNative(payable(user));

        uint256 beforeBalance = admin.balance;
        vm.prank(admin);
        DepositRouter(router).sweepNative(payable(admin));
        assertEq(admin.balance, beforeBalance + 1 ether, "owner should receive swept native token");
    }
}
